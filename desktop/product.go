package main

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/markdown"
	"github.com/lobehub/lobe-mygo/pkg/overlay"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

// 产品页：和网页版 product.tsx 一样，左边一列分组导航（概览 / 对话 / 方案 / 记录），右边是内容。

type navEntry struct{ key, label string }

var productNav = []struct {
	group string
	items []navEntry
}{
	{"", []navEntry{{"overview", "概览"}}},
	{"对话", []navEntry{{"answers", "回答问题"}, {"play", "试玩"}, {"revise", "加新需求"}}},
	{"方案", []navEntry{{"components", "组件选型"}, {"architecture", "架构与工具"}, {"prompt", "提示词与策略"}, {"review", "审稿意见"}}},
	{"记录", []navEntry{{"history", "版本历史"}}},
}

var chatTabs = map[string]bool{"answers": true, "play": true, "revise": true}

// setTab 切换产品页的分页；进试玩时拉开场白。
func setTab(slug, tab string) {
	app.mu.Lock()
	app.productTab = tab
	app.mu.Unlock()
	app.invalidate()
	if tab == "play" {
		go app.startPlay(slug)
	}
}

func productView(c *ui.Context) {
	tk := theme.TokensOf(c)
	app.mu.Lock()
	selected := app.selected
	tab := app.productTab
	plan := app.plan
	planErr := app.planErr
	job := app.job
	ver := app.prdVer
	path := app.productPath
	handoffBusy, handoffStatus := app.handoffBusy, app.handoffStatus
	name := ""
	for _, p := range app.products {
		if p.Slug == selected {
			name = p.Name
		}
	}
	app.mu.Unlock()

	if selected == "" {
		homeView(c)
		return
	}
	if name == "" {
		name = selected
	}

	pageShell(c, func() {
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, name).FontSize(15).Bold().MaxLines(1)
			if ver > 0 {
				mono(c, fmt.Sprintf("V%d", ver), 12)
			}
		})
		if handoffBusy {
			muted(c, "正在打包…", 12).MaxLines(1)
		} else if handoffStatus != "" {
			muted(c, handoffStatus, 12).MaxLines(1)
		}
		open := action.Button(c, "打开文件夹", action.ButtonProps{Size: theme.Small})
		open.OnClick(func() { go revealProduct(app.baseURL(), selected) })
		overlay.Tooltip(c, open, path, overlay.TooltipProps{})
		if plan != nil {
			action.Button(c, "导出交接包", action.ButtonProps{Variant: action.Primary, Size: theme.Small}).
				OnClick(func() { go app.downloadHandoffZip(selected) })
		}
	}, func() {
		ui.Row(c).Grow(1).Basis(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			productSubNav(c, selected, tab, plan)
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
				switch {
				case planErr != "":
					readingScroll(c, 16, func() { errorAlert(c, planErr) })
				case plan == nil:
					noPlanView(c, selected, job)
				case chatTabs[tab]:
					if job != nil && job.Status == "running" {
						ui.Row(c).Shrink(0).Padding(8, 20).Gap(4).AlignItems(ui.Center).
							BorderWidth(0, 0, 1, 0).BorderColor(tk.BorderSecondary).Children(func() {
							muted(c, "正在推导新方案，进度在", 12)
							action.Button(c, "概览", action.ButtonProps{Variant: action.Link, Size: theme.Small}).
								OnClick(func() { setTab(selected, "overview") })
							muted(c, "里看。", 12)
						})
					}
					switch tab {
					case "answers":
						answersView(c, selected, plan, job)
					case "revise":
						reviseView(c, selected, ver, job)
					default:
						playPanel(c, selected)
					}
				default:
					productScroll(c, tab == "overview", func() {
						modeCallouts(c)
						if job != nil && job.Status != "done" {
							jobProgress(c, job, planStages, planWording, false, func() { go app.startPlan(selected) })
							if job.Status == "running" {
								callout(c, feedback.Info, "下面还是上一版方案", "新方案推导完成后会自动刷新。", nil)
							}
						}
						switch tab {
						case "components":
							componentsTab(c, plan)
						case "architecture":
							architectureTab(c, plan)
						case "prompt":
							promptTab(c, plan)
						case "review":
							reviewTab(c, plan)
						case "history":
							historyTab(c, selected, plan)
						default:
							overviewTab(c, selected, plan)
						}
					})
				}
			})
		})
		prdDialog(c)
	})
}

// productScroll 非对话分页的滚动区：概览要放右侧栏，宽一些；其余是居中的阅读列。
func productScroll(c *ui.Context, wide bool, fn func()) {
	width := float32(chatWidth)
	if wide {
		width = 1040
	}
	ui.Scroll(c).Grow(1).Basis(0).Children(func() {
		ui.Row(c).Shrink(0).Justify(ui.Center).Padding(0, 28).Children(func() {
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).MaxWidth(width).Padding(24, 0, 40, 0).Gap(20).Children(fn)
		})
	})
}

// productSubNav 左侧分组导航 + 底部用量。
func productSubNav(c *ui.Context, slug, tab string, plan *PlanView) {
	tk := theme.TokensOf(c)
	counts := map[string]int{}
	if plan != nil {
		counts["answers"] = len(plan.Unknowns)
		counts["architecture"] = len(plan.ToolList)
		if plan.Review != nil {
			counts["review"] = len(plan.Review.Issues)
		}
	}
	ui.Column(c).Width(188).Shrink(0).Padding(12, 8).Gap(2).
		BorderWidth(0, 1, 0, 0).BorderColor(tk.BorderSecondary).Children(func() {
		ui.Column(c).Grow(1).Basis(0).Gap(2).Children(func() {
			for _, g := range productNav {
				if g.group != "" {
					muted(c, g.group, 12).Padding(12, 10, 4, 10)
				}
				for _, it := range g.items {
					row := ui.ButtonBase(c).Row().Height(32).Padding(0, 10).Radius(8).
						AlignItems(ui.Center).Justify(ui.SpaceBetween).Cursor(ui.CursorPointer)
					active := it.key == tab
					switch {
					case active:
						row.Background(tk.FillSecondary)
					case row.Hovered():
						row.Background(tk.FillQuaternary)
					}
					row.Children(func() {
						label := ui.Text(c, it.label).FontSize(14)
						if active {
							label.FontWeight(600)
						} else {
							label.TextColor(tk.TextSecondary)
						}
						if n := counts[it.key]; n > 0 {
							mono(c, fmt.Sprint(n), 12)
						}
					})
					row.OnClick(func() { setTab(slug, it.key) })
				}
			}
		})
		usageFooter(c, plan)
	})
}

// usageFooter 用量：有累计就用累计，没有就用最近一轮推导的数字，都是 0 就不显示。
func usageFooter(c *ui.Context, plan *PlanView) {
	if plan == nil {
		return
	}
	app.mu.Lock()
	u := app.usage
	app.mu.Unlock()
	label, n := "累计", u.Total
	if u.Total.Calls == 0 {
		label, n = "最近一轮推导", plan.Budget
	}
	if n.Calls == 0 {
		return
	}
	model := "真实模型"
	switch {
	case plan.Runner == "mock":
		model = "离线演示"
	case plan.Model != "":
		model = "模型 " + plan.Model
	}
	lines := []string{model, fmt.Sprintf("%s调用 %s 次", label, fmtNum(n.Calls)),
		fmt.Sprintf("输入 %s tokens", fmtNum(n.InputTokens)), fmt.Sprintf("输出 %s tokens", fmtNum(n.OutputTokens))}
	if n.CacheReadTokens > 0 {
		lines = append(lines, fmt.Sprintf("命中缓存 %s tokens", fmtNum(n.CacheReadTokens)))
	}
	attempts := plan.DesignAttempts
	if attempts == 0 {
		attempts = 1
	}
	lines = append(lines, fmt.Sprintf("设计稿 %d 版", attempts))
	tk := theme.TokensOf(c)
	box := ui.Column(c).Shrink(0).Gap(2).Padding(10, 10, 4, 10).BorderWidth(1, 0, 0, 0).BorderColor(tk.BorderSecondary)
	box.Children(func() {
		for _, l := range lines {
			muted(c, l, 12)
		}
	})
	if u.Total.Calls > 0 {
		overlay.Tooltip(c, box, fmt.Sprintf("其中对话 %d 次；新建产品时的三轮对话不计在内", u.Chat.Calls), overlay.TooltipProps{})
	}
}

// noPlanView 还没有方案：推导中就看进度，没在推导就给「生成方案」。
func noPlanView(c *ui.Context, slug string, job *JobInfo) {
	app.mu.Lock()
	busy := app.planBusy
	app.mu.Unlock()
	productScroll(c, false, func() {
		modeCallouts(c)
		if job != nil && job.Status != "done" {
			jobProgress(c, job, planStages, planWording, false, func() { go app.startPlan(slug) })
			return
		}
		if job == nil {
			callout(c, feedback.Info, "还没有方案", "上次推导可能被中断了。重新推导时，已完成的阶段不会重复调用模型。", func() {
				b := action.Button(c, "生成方案", action.ButtonProps{Variant: action.Primary, Size: theme.Small})
				b.Disabled(busy || !configured())
				if !busy && configured() {
					b.OnClick(func() { go app.startPlan(slug) })
				}
			})
		}
	})
}

// -- 概览 ----------------------------------------------------------------------

var componentQuestion = regexp.MustCompile(`^(.*)（关系到组件 (.+)）$`)

func reviewChip(c *ui.Context, r *PlanReview) {
	if r == nil {
		return
	}
	score := ""
	if r.Score != nil {
		score = fmt.Sprintf(" · %d 分", int(*r.Score))
	}
	if r.OK {
		chip(c, "审稿通过"+score, toneOk)
	} else {
		chip(c, "审稿未通过"+score, toneWarn)
	}
}

func overviewTab(c *ui.Context, slug string, plan *PlanView) {
	tk := theme.TokensOf(c)
	ui.Row(c).Gap(24).AlignItems(ui.Start).Children(func() {
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(28).Children(func() {
			// 产品定位
			section(c, func() {
				sectionTitle(c, "产品定位")
				reviewChip(c, plan.Review)
			}, nil, func() {
				text := plan.OneLiner
				if text == "" {
					text = plan.Title
				}
				ui.Text(c, text).FontSize(16).LineHeight(1.7)
			})

			// 待确认问题
			qs := plan.Questions
			section(c, func() {
				sectionTitle(c, fmt.Sprintf("待确认问题（%d）", len(qs)))
			}, func() {
				if len(qs) > 0 {
					action.Button(c, "开始回答 →", action.ButtonProps{Variant: action.Primary, Size: theme.Small}).
						OnClick(func() { setTab(slug, "answers") })
				}
			}, func() {
				if len(qs) == 0 {
					muted(c, "没有需要你回答的问题了。", 14)
					return
				}
				muted(c, "答不上来的就是风险点。点「开始回答」在对话里一题一题过，拿不准的可以让 AI 给你几个方向；答完自动重新推导。", 13)
				key := 0
				for _, q := range qs {
					if q.Priority == 1 {
						key++
					}
				}
				if key > 0 {
					muted(c, fmt.Sprintf("其中 %d 个关键问题排在最前面：不回答就没法动手，或者会让方案大改。", key), 13)
				}
				expanded := ui.Local(ui.Column(c), "q-expanded", func() bool { return false })
				shown := qs
				if !*expanded && len(qs) > 8 {
					shown = qs[:8]
				}
				divided(c, len(shown), func(i int) {
					q := shown[i]
					row := ui.Row(c).Gap(10).AlignItems(ui.Start)
					row.Children(func() {
						mono(c, pad2(i+1), 12).Width(22).Shrink(0).Padding(2, 0, 0, 0)
						if q.Priority == 1 {
							chip(c, "关键", toneMuted)
						}
						ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(6).Wrap().AlignItems(ui.Center).Children(func() {
							text, comp := q.Text, ""
							if m := componentQuestion.FindStringSubmatch(q.Text); m != nil {
								text, comp = m[1], m[2]
							}
							ui.Text(c, text).FontSize(14)
							if comp != "" {
								mono(c, comp, 12)
							}
						})
					})
					if q.Original != "" && q.Original != q.Text {
						overlay.Tooltip(c, row, "原问题："+q.Original, overlay.TooltipProps{})
					}
				})
				if len(qs) > 8 {
					label := fmt.Sprintf("还有 %d 个问题，展开", len(qs)-8)
					if *expanded {
						label = "收起"
					}
					action.Button(c, label, action.ButtonProps{Variant: action.Text, Size: theme.Small}).
						OnClick(func() { *expanded = !*expanded; app.invalidate() })
				}
				if len(plan.DevItems) > 0 {
					display.Accordion(c, nil, display.AccordionProps{Items: []display.AccordionItem{{Key: "dev",
						Title: fmt.Sprintf("交给开发者的技术备注 %d · 你不用回答，交接包里会带给开发者", len(plan.DevItems))}}},
						func(string) {
							for i, d := range plan.DevItems {
								ui.Text(c, fmt.Sprintf("%s. %s", pad2(i+1), d.Text)).FontSize(13).TextColor(tk.TextSecondary)
							}
						})
				}
			})

			// 需求拆解
			if len(plan.Facts) > 0 {
				section(c, func() { sectionTitle(c, "需求拆解") }, nil, func() {
					tableHeader(c, []string{"维度", "结论", "状态"}, []float32{88, 0, 72})
					divided(c, len(plan.Facts), func(i int) {
						f := plan.Facts[i]
						ui.Row(c).Gap(12).AlignItems(ui.Start).Children(func() {
							ui.Text(c, f.Label).FontSize(14).FontWeight(600).Width(88).Shrink(0)
							v := f.Value
							if v == "" {
								v = "（空）"
							}
							ui.Text(c, v).FontSize(14).Grow(1).Basis(0).MinWidth(0)
							ui.Row(c).Width(72).Shrink(0).Children(func() {
								t := toneAsk
								if f.Status == "明确" {
									t = toneOk
								}
								chip(c, f.Status, t)
							})
						})
					})
				})
			}
		})

		// 右侧栏
		ui.Column(c).Width(272).Shrink(0).Gap(16).Children(func() {
			card(c, func() {
				ui.Row(c).AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
					muted(c, "组件选型", 13)
					action.Button(c, "看详情 →", action.ButtonProps{Variant: action.Text, Size: theme.Small}).
						OnClick(func() { setTab(slug, "components") })
				})
				for _, d := range decisionOrder {
					n := 0
					for _, comp := range plan.Components {
						if comp.Decision == d {
							n++
						}
					}
					row := ui.Row(c).AlignItems(ui.Center).Justify(ui.SpaceBetween)
					row.Children(func() {
						chip(c, d, decisionTone(d))
						mono(c, fmt.Sprint(n), 14).TextColor(tk.Text)
					})
					overlay.Tooltip(c, row, decisionHint[d], overlay.TooltipProps{})
				}
			})
			card(c, func() {
				must := 0
				for _, comp := range plan.Components {
					if comp.Decision == "必选" {
						must++
					}
				}
				muted(c, "下一步：交给会写代码的人", 12)
				ui.Text(c, "方案定下来以后，导出一个交接包（zip 文件），发给帮你做产品的开发者。").FontSize(14).LineHeight(1.6)
				ui.Column(c).Gap(2).Children(func() {
					for _, l := range []string{"交接说明.md · 从这里开始", "给AI编程工具的提示词.md", "PRD.md · 方案.md · 迭代记录.md",
						fmt.Sprintf("参考组件/ · %d 个必选组件", must)} {
						muted(c, l, 12)
					}
				})
				if n := len(plan.Questions); n > 0 {
					ui.Column(c).Padding(8, 10).Radius(8).Background(tk.InfoBg).Children(func() {
						ui.Text(c, fmt.Sprintf("还有 %d 个问题没回答。建议先答完再导出；也可以先导出，开发者会把这些问题带回来问你。", n)).
							FontSize(12).TextColor(tk.InfoText)
					})
				}
				action.Button(c, "导出交接包", action.ButtonProps{Variant: action.Primary}).
					OnClick(func() { go app.downloadHandoffZip(slug) })
				muted(c, "对方用 AI 编程工具的话，把「给AI编程工具的提示词.md」交给它就行。方案每更新一次，记得重新导出。", 12)
			})
		})
	})
}

// tableHeader 表头：一行灰色小字。width 为 0 的列占满剩下的宽度。
func tableHeader(c *ui.Context, cols []string, widths []float32) {
	tk := theme.TokensOf(c)
	ui.Row(c).Gap(12).Padding(0, 0, 6, 0).BorderWidth(0, 0, 1, 0).BorderColor(tk.BorderSecondary).Children(func() {
		for i, col := range cols {
			t := muted(c, col, 12)
			if widths[i] > 0 {
				t.Width(widths[i]).Shrink(0)
			} else {
				t.Grow(1).Basis(0)
			}
		}
	})
}

// -- 方案的四个分页 --------------------------------------------------------------

func componentsTab(c *ui.Context, plan *PlanView) {
	tk := theme.TokensOf(c)
	muted(c, "引擎内置 17 个 Harness 组件。选不选由固定规则决定：模型只判断需求有没有某个特征，同样的判断永远得到同样的选型。原则是只选需要的。", 13)
	for _, d := range decisionOrder {
		var group []PlanComponent
		for _, comp := range plan.Components {
			if comp.Decision == d {
				group = append(group, comp)
			}
		}
		if len(group) == 0 {
			continue
		}
		section(c, func() {
			chip(c, d, decisionTone(d))
			mono(c, fmt.Sprint(len(group)), 13)
		}, func() { muted(c, decisionHint[d], 12) }, func() {
			divided(c, len(group), func(i int) {
				comp := group[i]
				ui.Row(c).Gap(12).AlignItems(ui.Start).Children(func() {
					mono(c, comp.ID, 12).Width(34).Shrink(0).Padding(2, 0, 0, 0)
					ui.Column(c).Width(150).Shrink(0).Gap(2).Children(func() {
						name := ui.Text(c, comp.Name).FontSize(14).FontWeight(600)
						if d == "放弃" {
							name.TextColor(tk.TextTertiary)
						}
						muted(c, comp.Domain, 12)
					})
					ui.Text(c, comp.Reason).FontSize(13).TextColor(tk.TextSecondary).Grow(1).Basis(0).MinWidth(0)
				})
			})
		})
	}
}

func architectureTab(c *ui.Context, plan *PlanView) {
	tk := theme.TokensOf(c)
	section(c, func() { sectionTitle(c, "七层架构") }, nil, func() {
		divided(c, len(plan.Layers), func(i int) {
			l := plan.Layers[i]
			row := ui.Row(c).Gap(14).AlignItems(ui.Start).Padding(4, 8).Radius(8)
			if l.Num == "③" {
				row.Background(tk.FillQuaternary)
			}
			row.Children(func() {
				ui.Column(c).Width(220).Shrink(0).Gap(2).Children(func() {
					ui.Text(c, l.Num+" "+l.Name).FontSize(14).FontWeight(600)
					if l.Question != "" {
						muted(c, l.Question, 12)
					}
				})
				ui.Text(c, l.Detail).FontSize(13).TextColor(tk.TextSecondary).Grow(1).Basis(0).MinWidth(0)
			})
		})
	})
	section(c, func() { sectionTitle(c, "工具清单") }, func() { mono(c, fmt.Sprint(len(plan.ToolList)), 13) }, func() {
		if len(plan.ToolList) == 0 {
			muted(c, "（没有生成工具清单）", 13)
			return
		}
		for _, t := range plan.ToolList {
			card(c, func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
					ui.Text(c, t.Name).Font("monospace").FontSize(13).FontWeight(600)
					switch t.Permission {
					case "allow":
						chip(c, "直接执行", toneOk)
					case "ask":
						chip(c, "需人确认", toneWarn)
					case "deny":
						chip(c, "禁止", toneBad)
					default:
						chip(c, t.Permission, toneMuted)
					}
					if t.Layer != "" {
						muted(c, "第 "+t.Layer+" 层", 12)
					}
				})
				for _, kv := range [][2]string{{"输入", t.Input}, {"输出", t.Output}, {"副作用", t.SideEffect}} {
					v := kv[1]
					if v == "" {
						v = "—"
					}
					ui.Row(c).Gap(10).AlignItems(ui.Start).Children(func() {
						muted(c, kv[0], 12).Width(44).Shrink(0)
						ui.Text(c, v).FontSize(13).TextColor(tk.TextSecondary).Grow(1).Basis(0).MinWidth(0)
					})
				}
			})
		}
	})
	section(c, func() { sectionTitle(c, "五要素") }, nil, func() {
		divided(c, len(plan.Elements), func(i int) {
			e := plan.Elements[i]
			ui.Row(c).Gap(12).AlignItems(ui.Start).Children(func() {
				ui.Text(c, e.Label).FontSize(14).FontWeight(600).Width(88).Shrink(0)
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(4).Children(func() {
					if len(e.Items) == 0 {
						muted(c, "（待补充）", 13)
						return
					}
					bulletList(c, e.Items)
				})
			})
		})
	})
}

func promptTab(c *ui.Context, plan *PlanView) {
	tk := theme.TokensOf(c)
	section(c, func() { sectionTitle(c, "系统提示词草案") }, func() {
		if plan.SystemPrompt != "" {
			action.CopyButton(c, plan.SystemPrompt, action.CopyButtonProps{})
		}
	}, func() {
		text := plan.SystemPrompt
		if text == "" {
			text = "（空）"
		}
		ui.Column(c).Padding(12, 14).Radius(8).Background(tk.FillQuaternary).Children(func() {
			ui.Text(c, text).Font("monospace").FontSize(13).TextColor(tk.TextSecondary).LineHeight(1.6)
		})
	})
	section(c, func() { sectionTitle(c, "上下文与记忆策略") }, nil, func() {
		markdown.Markdown(c, breaks(orEmpty(plan.ContextMemoryStrategy)), markdown.MarkdownProps{})
	})
	section(c, func() { sectionTitle(c, "安全边界与降级策略") }, nil, func() { bulletsOrEmpty(c, plan.SecurityAndDegradation) })
	section(c, func() { sectionTitle(c, "可观测性") }, nil, func() {
		o := plan.Observability
		for _, row := range []struct {
			label string
			body  func()
		}{
			{"日志", func() { bulletsOrEmpty(c, o.Logs) }},
			{"指标", func() { bulletsOrEmpty(c, o.Metrics) }},
			{"轨迹", func() { markdown.Markdown(c, breaks(orEmpty(o.Trajectory)), markdown.MarkdownProps{}) }},
		} {
			ui.Row(c).Gap(12).AlignItems(ui.Start).Children(func() {
				ui.Text(c, row.label).FontSize(14).FontWeight(600).Width(44).Shrink(0)
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(4).Children(row.body)
			})
		}
	})
	section(c, func() { sectionTitle(c, "技术栈与部署形态") }, nil, func() {
		markdown.Markdown(c, breaks(orEmpty(plan.TechStack)), markdown.MarkdownProps{})
	})
	if len(plan.Testing) > 0 {
		section(c, func() { sectionTitle(c, "测试设计") }, nil, func() { bulletList(c, plan.Testing) })
	}
}

func orEmpty(s string) string {
	if strings.TrimSpace(s) == "" {
		return "（空）"
	}
	return s
}

func bulletsOrEmpty(c *ui.Context, items []string) {
	if len(items) == 0 {
		muted(c, "（空）", 13)
		return
	}
	bulletList(c, items)
}

func reviewTab(c *ui.Context, plan *PlanView) {
	tk := theme.TokensOf(c)
	r := plan.Review
	if r == nil {
		muted(c, "这份方案没有审稿记录。", 14)
		return
	}
	section(c, func() {
		sectionTitle(c, "独立审稿")
		reviewChip(c, r)
	}, nil, func() {
		if r.Summary != "" {
			ui.Text(c, r.Summary).FontSize(14).LineHeight(1.7)
		}
		muted(c, "审稿经常给出「未通过」，这是设计如此：审稿就是要把方案里的矛盾挑出来，这些意见就是你下一轮要想清楚的问题。", 13)
	})
	if len(r.Issues) > 0 {
		section(c, func() { sectionTitle(c, "意见") }, func() { mono(c, fmt.Sprint(len(r.Issues)), 13) }, func() {
			divided(c, len(r.Issues), func(i int) {
				is := r.Issues[i]
				ui.Row(c).Gap(10).AlignItems(ui.Start).Children(func() {
					ui.Row(c).Width(48).Shrink(0).Children(func() {
						switch is.Severity {
						case "high":
							chip(c, "严重", toneBad)
						case "medium":
							chip(c, "中等", toneWarn)
						default:
							chip(c, "轻微", toneMuted)
						}
					})
					ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(3).Children(func() {
						ui.Text(c, is.Section).FontSize(14).FontWeight(600)
						ui.Text(c, is.Problem).FontSize(13).TextColor(tk.TextSecondary).LineHeight(1.6)
					})
				})
			})
		})
	}
}

// -- 版本历史 --------------------------------------------------------------------

var historyKind = map[string]string{
	"建立工作区": "建立产品", "方案推导": "推导方案", "新需求": "加新需求", "回答待确认问题": "回答问题", "验证": "检查代码骨架",
}

var (
	reReq      = regexp.MustCompile(`^(新需求|原始需求)：(.+)$`)
	reRejudge  = regexp.MustCompile(`^重新判断：(.+)；其余沿用 (V\d+) 的判断$`)
	reChange   = regexp.MustCompile(`^选型变化：(.+)$`)
	reChangeIt = regexp.MustCompile(`^(s\d\d) (\S+) → (\S+)$`)
	reMust     = regexp.MustCompile(`^必选组件：(.+)$`)
	reReview   = regexp.MustCompile(`^方案：.*审稿(通过|未完全通过)`)
	rePrd      = regexp.MustCompile(`^PRD：(V\d+) → (V\d+)`)
	reAnswered = regexp.MustCompile(`^回答了 (\d+) 个问题`)
	reVerify   = regexp.MustCompile(`^对照 PRD (V\d+) 检查 skeleton/：(.+)$`)
)

var historyHidden = []string{"`blueprint", "人工验证结论", "等 AI 消化", "骨架待办", "下一步", "快速模式", "本轮未重推", "报告：", "待确认："}

// historyLine 按网页版 describe 的规则显示迭代记录里的一行；不该给人看的返回 false。
func historyLine(c *ui.Context, line string, names map[string]string) {
	tk := theme.TokensOf(c)
	line = strings.TrimSpace(line)
	for _, h := range historyHidden {
		if strings.Contains(line, h) {
			return
		}
	}
	text := func(s string) { ui.Text(c, s).FontSize(13).TextColor(tk.TextSecondary).LineHeight(1.6) }
	switch {
	case reReq.MatchString(line):
		v := strings.TrimSpace(strings.TrimLeft(reReq.FindStringSubmatch(line)[2], "# "))
		if v == "一句话想法" || v == "要加什么或改什么" {
			return
		}
		ui.Text(c, v).FontSize(14).FontWeight(600)
	case strings.HasPrefix(line, "选型变化：无"):
		text("组件选型没有变化")
	case reRejudge.MatchString(line):
		m := reRejudge.FindStringSubmatch(line)
		head := "这次只重新判断了：" + m[1]
		if m[1] == "无" {
			head = "新需求没涉及已有组件的判断"
		}
		text(fmt.Sprintf("%s；其余沿用 %s 的判断，避免和新需求无关的选型跟着变。", head, m[2]))
	case reChange.MatchString(line):
		parts := strings.Split(reChange.FindStringSubmatch(line)[1], "；")
		text(fmt.Sprintf("组件选型变了 %d 处：", len(parts)))
		for _, p := range parts {
			m := reChangeIt.FindStringSubmatch(strings.TrimSpace(p))
			if m == nil {
				text(p)
				continue
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(0, 0, 0, 8).Children(func() {
				mono(c, m[1], 12)
				ui.Text(c, names[m[1]]).FontSize(13)
				chip(c, m[2], decisionTone(m[2]))
				muted(c, "→", 12)
				chip(c, m[3], decisionTone(m[3]))
			})
		}
	case reMust.MatchString(line):
		ids := strings.Split(reMust.FindStringSubmatch(line)[1], "、")
		for i, id := range ids {
			id = strings.TrimSpace(id)
			if n := names[id]; n != "" {
				ids[i] = id + " " + n
			}
		}
		text("第一版要做：" + strings.Join(ids, "、"))
	case reReview.MatchString(line):
		if reReview.FindStringSubmatch(line)[1] == "通过" {
			text("独立审稿通过")
		} else {
			text("独立审稿提了意见，见「审稿意见」")
		}
	case rePrd.MatchString(line):
		m := rePrd.FindStringSubmatch(line)
		text(fmt.Sprintf("需求文档 %s → %s", m[1], m[2]))
	case reAnswered.MatchString(line):
		text(fmt.Sprintf("回答了 %s 个问题，写进了需求文档", reAnswered.FindStringSubmatch(line)[1]))
	case reVerify.MatchString(line):
		m := reVerify.FindStringSubmatch(line)
		text(fmt.Sprintf("对照 %s 检查代码骨架：%s", m[1], m[2]))
	case strings.HasPrefix(line, "⚠️"):
		text(line)
	}
}

func historyTab(c *ui.Context, slug string, plan *PlanView) {
	tk := theme.TokensOf(c)
	app.mu.Lock()
	h, herr := app.history, app.historyErr
	key := fmt.Sprintf("%s:%d", slug, app.prdVer)
	for _, p := range app.products {
		if p.Slug == slug {
			key += fmt.Sprintf(":%.0f", p.Updated)
		}
	}
	stale := app.historyFor != key
	app.mu.Unlock()
	if stale {
		go app.loadHistory(slug, key)
	}
	if herr != "" {
		callout(c, feedback.Error, "读不到版本历史", herr, nil)
		return
	}
	if h == nil {
		mutedText(c, "读取中…")
		return
	}
	names := map[string]string{}
	for _, comp := range plan.Components {
		names[comp.ID] = comp.Name
	}
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Text(c, fmt.Sprintf("当前版本 V%d", h.Current)).FontSize(18).FontWeight(600).Grow(1)
		action.Button(c, "看当前的需求文档", action.ButtonProps{Size: theme.Small}).OnClick(func() { go app.openPrd(slug, 0) })
	})
	if len(h.Versions) > 1 {
		section(c, func() { muted(c, "版本记录", 12) }, nil, func() {
			vs := slices.Clone(h.Versions)
			slices.Reverse(vs)
			divided(c, len(vs), func(i int) {
				v := vs[i]
				row := ui.Row(c).Gap(12).AlignItems(ui.Start).Padding(2, 6).Radius(6)
				if v.Version == h.Current {
					row.Background(tk.FillQuaternary)
				}
				row.Children(func() {
					mono(c, fmt.Sprintf("V%d", v.Version), 13).TextColor(tk.Text).Width(36).Shrink(0)
					muted(c, v.Date, 12).Width(84).Shrink(0)
					ui.Text(c, v.Change).FontSize(13).Grow(1).Basis(0).MinWidth(0)
					muted(c, v.Source, 12).Shrink(0)
				})
			})
		})
	}
	section(c, func() { muted(c, "每一轮做了什么", 12) }, nil, func() {
		canOpen := map[int]bool{h.Current: true}
		for _, s := range h.Snapshots {
			canOpen[s] = true
		}
		seen := map[int]bool{}
		log := slices.Clone(h.Log)
		slices.Reverse(log)
		for _, e := range log {
			card(c, func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
					muted(c, e.Date, 12)
					if e.Version != nil {
						mono(c, fmt.Sprintf("V%d", *e.Version), 12)
					}
					kind := historyKind[e.Title]
					if kind == "" {
						kind = e.Title
					}
					ui.Text(c, kind).FontSize(14).FontWeight(600).Grow(1)
					if e.Version != nil && !seen[*e.Version] && canOpen[*e.Version] && (e.Title == "建立工作区" || e.Title == "新需求") {
						seen[*e.Version] = true
						v := *e.Version
						action.Button(c, "看当时的需求文档", action.ButtonProps{Variant: action.Text, Size: theme.Small}).OnClick(func() {
							if v == h.Current {
								v = 0
							}
							go app.openPrd(slug, v)
						})
					}
				})
				for _, l := range e.Lines {
					historyLine(c, l, names)
				}
			})
		}
	})
}

// prdDialog 某一版需求文档的弹窗。
func prdDialog(c *ui.Context) {
	app.mu.Lock()
	open, doc, derr := app.prdDocOpen, app.prdDoc, app.prdDocErr
	app.mu.Unlock()
	before := open
	title := "需求文档"
	if doc != nil {
		title = fmt.Sprintf("需求文档 V%d", doc.Version)
		if doc.Current {
			title += "（当前）"
		}
	}
	feedback.Modal(c, &open, feedback.ModalProps{Title: title, Width: 760, NoFooter: true}, func() {
		ui.Scroll(c).MaxHeight(560).Children(func() {
			ui.Column(c).Padding(0, 4, 8, 0).Children(func() {
				switch {
				case derr != "":
					errorAlert(c, derr)
				case doc == nil:
					mutedText(c, "读取中…")
				default:
					markdown.Markdown(c, breaks(doc.Text), markdown.MarkdownProps{})
				}
			})
		})
	})
	if open != before {
		app.mu.Lock()
		app.prdDocOpen = open
		app.mu.Unlock()
	}
}

// -- 首页：我的产品 -------------------------------------------------------------

func homeView(c *ui.Context) {
	tk := theme.TokensOf(c)
	app.mu.Lock()
	products := slices.Clone(app.products)
	app.mu.Unlock()
	slices.SortStableFunc(products, func(a, b Product) int {
		switch {
		case a.Updated > b.Updated:
			return -1
		case a.Updated < b.Updated:
			return 1
		}
		return 0
	})
	gotoNew := func() {
		app.mu.Lock()
		app.page = PageNew
		app.mu.Unlock()
		app.invalidate()
	}
	pageShell(c, func() {
		ui.Row(c).Grow(1).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "我的产品").FontSize(15).Bold()
			if len(products) > 0 {
				mono(c, fmt.Sprint(len(products)), 12)
			}
		})
		if len(products) > 0 {
			action.Button(c, "新建产品", action.ButtonProps{Variant: action.Primary, Size: theme.Small}).OnClick(gotoNew)
		}
	}, func() {
		productScroll(c, false, func() {
			modeCallouts(c)
			if len(products) == 0 {
				ui.Column(c).Gap(10).Padding(24, 0, 0, 0).Children(func() {
					ui.Text(c, "把一个想法，推导成能做的 Agent 方案").FontSize(26).Bold()
					ui.Text(c, "不用懂技术，也不用会写需求文档。像聊天一样说清楚你想做什么，剩下的交给引擎。").FontSize(15).TextColor(tk.TextSecondary)
				})
				divided(c, 3, func(i int) {
					steps := [][2]string{
						{"说出想法", "一句话就行。我会分三轮问你：给谁用、怎么用、什么不能做。"},
						{"引擎推导方案", "需求拆解、组件选型、架构和提示词，最后还有一轮独立审稿。"},
						{"回答问题，再迭代", "方案会列出还没想清楚的问题，在对话里一题一题答，答完自动重新推导。"},
					}
					ui.Row(c).Gap(14).AlignItems(ui.Start).Children(func() {
						mono(c, pad2(i+1), 13).Shrink(0)
						ui.Column(c).Gap(2).Children(func() {
							ui.Text(c, steps[i][0]).FontSize(15).FontWeight(600)
							muted(c, steps[i][1], 13)
						})
					})
				})
				ui.Row(c).Children(func() {
					action.Button(c, "开始一个新产品 →", action.ButtonProps{Variant: action.Primary}).OnClick(gotoNew)
				})
				return
			}
			ui.Column(c).Children(func() {
				tableHeader(c, []string{"版本", "产品", "状态", "更新"}, []float32{48, 0, 140, 88})
				for _, p := range products {
					row := ui.ButtonBase(c).Row().Gap(12).Padding(12, 0).AlignItems(ui.Center).Cursor(ui.CursorPointer).
						BorderWidth(0, 0, 1, 0).BorderColor(tk.BorderSecondary)
					if row.Hovered() {
						row.Background(tk.FillQuaternary)
					}
					row.Children(func() {
						mono(c, fmt.Sprintf("V%d", p.Version), 12).Width(48).Shrink(0)
						ui.Text(c, p.Name).FontSize(14).FontWeight(500).Grow(1).Basis(0).MinWidth(0).MaxLines(1)
						ui.Row(c).Width(140).Shrink(0).Children(func() {
							switch {
							case !p.HasPlan:
								chip(c, "还没有方案", toneMuted)
							case p.Unknowns > 0:
								chip(c, fmt.Sprintf("%d 个待确认问题", p.Unknowns), toneAsk)
							default:
								chip(c, "方案已生成", toneOk)
							}
						})
						muted(c, timeAgo(p.Updated), 12).Width(88).Shrink(0)
					})
					row.OnClick(func() { go app.selectProduct(p.Slug) })
				}
			})
			muted(c, "每个产品一个文件夹，需求文档、方案、迭代记录都在引擎目录的 out/ 下。", 12)
		})
	})
}
