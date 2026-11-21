package main

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/markdown"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

// sidebar 左栏：导航（产品/新建/设置）+ 产品列表 + 引擎状态。
func sidebar(c *ui.Context) {
	tk := theme.TokensOf(c)
	app.mu.Lock()
	page := app.page
	products := append([]Product(nil), app.products...)
	selected := app.selected
	engineErr := app.engineErr
	up := app.engineURL != ""
	app.mu.Unlock()

	ui.Column(c).Width(268).Shrink(0).Padding(12).Gap(8).Children(func() {
		ui.Row(c).Height(32).Shrink(0).AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
			ui.Text(c, "Agent 工坊").FontSize(15).Bold()
			action.ActionIcon(c, lucide.RefreshCw(), action.ActionIconProps{Title: "刷新产品列表"}).OnClick(func() {
				go app.refreshProducts()
			})
		})

		// 导航
		navClicked := display.List(c, display.ListProps{Items: []display.ListItem{
			{Key: "products", Label: "产品", Icon: lucide.Boxes()},
			{Key: "new", Label: "新建产品", Icon: lucide.Sparkles()},
			{Key: "audits", Label: "项目检查", Icon: lucide.ScanSearch()},
			{Key: "settings", Label: "模型设置", Icon: lucide.SlidersHorizontal()},
		}, Active: navKeyOf(page)})
		switch navClicked {
		case "products":
			app.mu.Lock()
			app.page = PageProducts
			app.mu.Unlock()
		case "new":
			app.mu.Lock()
			app.page = PageNew
			app.mu.Unlock()
		case "audits":
			app.mu.Lock()
			app.page = PageAudits
			app.mu.Unlock()
			go app.loadAudits()
		case "settings":
			gotoSettings()
		}

		// 产品列表：单独一组，带小标题；产品多了自己滚动，不把底部状态挤出窗口
		ui.Text(c, "我的产品").FontSize(12).TextColor(tk.TextTertiary).Padding(10, 8, 0, 8)
		ui.Scroll(c).Grow(1).Basis(0).Children(func() {
			items := make([]display.ListItem, 0, len(products))
			for _, p := range products {
				desc := fmt.Sprintf("V%d · 待确认 %d", p.Version, p.Unknowns)
				if !p.HasPlan {
					desc = fmt.Sprintf("V%d · 还没有方案", p.Version)
				}
				items = append(items, display.ListItem{
					Key: p.Slug, Label: p.Name, Description: desc, Icon: lucide.Boxes(),
				})
			}
			if len(items) == 0 {
				ui.Text(c, "还没有产品，去「新建产品」聊一个出来。").FontSize(12).TextColor(tk.TextTertiary).Padding(4, 8)
				return
			}
			clicked := display.List(c, display.ListProps{Items: items, Active: selected})
			if clicked != "" {
				go app.selectProduct(clicked)
			}
		})

		// 底部：引擎状态（小圆点 + 一行字）
		status, dot, color := "引擎启动中…", tk.Warning, tk.TextTertiary
		switch {
		case engineErr != "":
			status, dot, color = engineErr, tk.Error, tk.ErrorText
		case up:
			status, dot = "引擎已连接", tk.Success
		}
		ui.Row(c).Shrink(0).Gap(8).Padding(6, 8, 2, 8).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Width(7).Height(7).Radius(4).Background(dot).Shrink(0)
			ui.Text(c, status).FontSize(12).TextColor(color).MaxLines(2)
		})
	})
}

func navKeyOf(page Page) string {
	switch page {
	case PageNew:
		return "new"
	case PageSettings:
		return "settings"
	case PageAudits:
		return "audits"
	default:
		return "products"
	}
}

// content 右栏：按当前页面渲染。
func content(c *ui.Context) {
	app.mu.Lock()
	page := app.page
	app.mu.Unlock()
	switch page {
	case PageNew:
		newProductView(c)
	case PageSettings:
		settingsView(c)
	case PageAudits:
		auditsView(c)
	default:
		productView(c)
	}
}

// productView 产品详情：标题栏中间是 PRD / 方案 / 试玩 分段切换，右边是交接包和工作区按钮。
func productView(c *ui.Context) {
	tk := theme.TokensOf(c)
	app.mu.Lock()
	selected := app.selected
	prdText := app.prdText
	prdVer := app.prdVer
	prdErr := app.prdErr
	loading := app.loading
	tab := app.productTab
	hasProducts := len(app.products) > 0
	handoffBusy := app.handoffBusy
	handoffStatus := app.handoffStatus
	name := ""
	for _, p := range app.products {
		if p.Slug == selected {
			name = p.Name
		}
	}
	app.mu.Unlock()

	pageShell(c, func() {
		if selected == "" {
			pageTitle(c, "产品", "")
			return
		}
		// 左右两块等宽，分段控件就落在正中
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, name).FontSize(15).Bold().MaxLines(1)
			if prdVer > 0 {
				display.Tag(c, fmt.Sprintf("V%d", prdVer), display.TagProps{Size: theme.Small})
			}
		})
		action.Segmented(c, &tab, []string{"PRD", "方案", "试玩"}, action.SegmentedProps{Size: theme.Small})
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(4).Justify(ui.End).AlignItems(ui.Center).Children(func() {
			if handoffBusy {
				ui.Text(c, "正在打包…").FontSize(12).TextColor(tk.TextTertiary).MaxLines(1)
			} else if handoffStatus != "" {
				ui.Text(c, handoffStatus).FontSize(12).TextColor(tk.TextTertiary).MaxLines(1)
			}
			action.ActionIcon(c, lucide.PackageOpen(), action.ActionIconProps{Title: "下载交接包"}).OnClick(func() {
				go app.downloadHandoffZip(selected)
			})
			action.ActionIcon(c, lucide.FolderOpen(), action.ActionIconProps{Title: "在 Finder 里打开工作区"}).OnClick(func() {
				go revealProduct(app.baseURL(), selected)
			})
		})
	}, func() {
		if selected == "" {
			if hasProducts {
				centerEmpty(c, display.EmptyProps{Icon: lucide.Boxes(), Title: "选一个产品",
					Description: "在左边选一个产品，这里会显示它的 PRD 和方案，还能直接试玩。"})
			} else {
				centerEmpty(c, display.EmptyProps{Icon: lucide.Sparkles(), Title: "还没有产品",
					Description: "先用对话把你的想法说清楚，引擎会帮你整理成第一个产品。",
					Action: func() {
						action.Button(c, "新建产品", action.ButtonProps{Variant: action.Primary}).OnClick(func() {
							app.mu.Lock()
							app.page = PageNew
							app.mu.Unlock()
						})
					}})
			}
			return
		}
		switch tab {
		case 0:
			prdPanel(c, loading, prdErr, prdText)
		case 1:
			planPanel(c)
		default:
			playPanel(c)
		}
	})

	app.mu.Lock()
	if tab != app.productTab && tab == 2 {
		go app.startPlay(selected)
	}
	app.productTab = tab
	app.mu.Unlock()
}

// prdPanel PRD 标签页：居中的阅读列。
func prdPanel(c *ui.Context, loading bool, prdErr, prdText string) {
	readingScroll(c, 8, func() {
		switch {
		case loading:
			mutedText(c, "读取中…")
		case prdErr != "":
			errorAlert(c, prdErr)
		default:
			markdown.Markdown(c, prdText, markdown.MarkdownProps{})
		}
	})
}

// planPanel 方案标签页：标题区 + 一节一张卡片（待确认、事实、组件、七层架构……）。
func planPanel(c *ui.Context) {
	tk := theme.TokensOf(c)
	app.mu.Lock()
	plan := app.plan
	planErr := app.planErr
	job := app.job
	app.mu.Unlock()

	readingScroll(c, 16, func() {
		if planErr != "" {
			errorAlert(c, planErr)
			return
		}
		if plan == nil {
			mutedText(c, "读取方案中…")
			return
		}

		if job != nil && job.Status != "" {
			jobBlock(c, job)
		}

		// 标题与定位
		ui.Column(c).Gap(8).Padding(0, 0, 6, 0).Children(func() {
			if plan.Title != "" {
				ui.Text(c, plan.Title).FontSize(22).Bold()
			}
			if plan.OneLiner != "" && plan.OneLiner != plan.Title {
				ui.Text(c, plan.OneLiner).FontSize(14).TextColor(tk.TextSecondary)
			}
			if plan.Runner != "" || plan.Model != "" {
				ui.Row(c).Gap(6).Children(func() {
					if plan.Runner != "" {
						display.Tag(c, plan.Runner, display.TagProps{Size: theme.Small})
					}
					if plan.Model != "" {
						display.Tag(c, plan.Model, display.TagProps{Size: theme.Small})
					}
				})
			}
		})

		if len(plan.Unknowns) > 0 {
			sectionCard(c, "待确认清单", len(plan.Unknowns), func() {
				for i, q := range plan.Unknowns {
					ui.Row(c).Gap(10).AlignItems(ui.Start).Children(func() {
						ui.Text(c, fmt.Sprintf("%d", i+1)).FontSize(13).TextColor(tk.TextTertiary).Width(20).Shrink(0)
						ui.Text(c, q).FontSize(14).Grow(1).Basis(0).MinWidth(0)
					})
				}
			})
		}

		if len(plan.Facts) > 0 {
			sectionCard(c, "已确认的事实", 0, func() {
				for _, f := range plan.Facts {
					ui.Row(c).Gap(10).AlignItems(ui.Start).Children(func() {
						ui.Text(c, f.Label).FontSize(13).TextColor(tk.TextSecondary).Width(96).Shrink(0)
						ui.Text(c, f.Value).FontSize(14).Grow(1).Basis(0).MinWidth(0)
						display.Tag(c, f.Status, display.TagProps{Color: factColor(f.Status), Size: theme.Small})
					})
				}
			})
		}

		if len(plan.Components) > 0 {
			sectionCard(c, "组件选型", len(plan.Components), func() {
				for _, comp := range plan.Components {
					ui.Row(c).Gap(10).AlignItems(ui.Start).Children(func() {
						ui.Column(c).Width(56).Shrink(0).Children(func() {
							display.Tag(c, comp.Decision, display.TagProps{Color: decisionColor(comp.Decision), Size: theme.Small})
						})
						ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(2).Children(func() {
							ui.Text(c, comp.OneLiner).FontSize(14).Bold()
							if comp.Reason != "" {
								ui.Text(c, comp.Reason).FontSize(13).TextColor(tk.TextSecondary)
							}
						})
					})
				}
			})
		}

		if len(plan.Layers) > 0 {
			sectionCard(c, "七层架构", 0, func() {
				for _, layer := range plan.Layers {
					ui.Column(c).Gap(3).Children(func() {
						ui.Text(c, fmt.Sprintf("%s %s", layer.Num, layer.Name)).FontSize(14).Bold()
						if layer.Detail != "" {
							ui.Text(c, layer.Detail).FontSize(13).TextColor(tk.TextSecondary)
						}
						if layer.Question != "" {
							ui.Text(c, "待确认："+layer.Question).FontSize(13).TextColor(tk.WarningText)
						}
					})
				}
			})
		}

		if len(plan.ToolList) > 0 {
			sectionCard(c, "工具清单", len(plan.ToolList), func() {
				for _, tool := range plan.ToolList {
					perm := map[string]string{"allow": "直接执行", "ask": "先问人", "deny": "禁止"}[tool.Permission]
					if perm == "" {
						perm = tool.Permission
					}
					ui.Row(c).Gap(10).AlignItems(ui.Start).Children(func() {
						ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(2).Children(func() {
							ui.Text(c, tool.Name).FontSize(14).Bold()
							ui.Text(c, tool.Input+" → "+tool.Output).FontSize(13).TextColor(tk.TextSecondary)
						})
						display.Tag(c, perm, display.TagProps{Size: theme.Small})
					})
				}
			})
		}

		if len(plan.SecurityAndDegradation) > 0 {
			sectionCard(c, "降级与安全", 0, func() { bulletList(c, plan.SecurityAndDegradation) })
		}
		if len(plan.Testing) > 0 {
			sectionCard(c, "测试设计", 0, func() { bulletList(c, plan.Testing) })
		}

		if plan.SystemPrompt != "" {
			sectionCard(c, "系统提示词草案", 0, func() {
				ui.Column(c).Padding(12, 14).Radius(8).Background(tk.FillQuaternary).Children(func() {
					ui.Text(c, plan.SystemPrompt).FontSize(13).TextColor(tk.TextSecondary)
				})
			})
		}

		if plan.Review != nil {
			ok, _ := plan.Review["ok"].(bool)
			sectionCard(c, "独立审稿", 0, func() {
				ui.Row(c).Children(func() {
					if ok {
						display.Tag(c, "审稿通过", display.TagProps{Color: display.TagSuccess})
					} else {
						display.Tag(c, "审稿未通过", display.TagProps{Color: display.TagWarning})
					}
				})
			})
		}
	})
}

// bulletList 一组要点：灰色圆点 + 正文，长句换行时和正文对齐。
func bulletList(c *ui.Context, items []string) {
	tk := theme.TokensOf(c)
	for _, item := range items {
		ui.Row(c).Gap(8).AlignItems(ui.Start).Children(func() {
			ui.Text(c, "•").FontSize(14).TextColor(tk.TextTertiary).Shrink(0)
			ui.Text(c, item).FontSize(14).Grow(1).Basis(0).MinWidth(0)
		})
	}
}

// jobBlock 任务进度块：状态标签 + 最近日志。
func jobBlock(c *ui.Context, job *JobInfo) {
	tk := theme.TokensOf(c)
	statusText, color := job.Status, display.TagProcessing
	switch job.Status {
	case "done":
		statusText, color = "完成", display.TagSuccess
	case "failed":
		statusText, color = "失败", display.TagError
	}
	ui.Column(c).Padding(12, 14).Gap(6).Radius(10).Background(tk.FillQuaternary).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			display.Tag(c, "推导任务", display.TagProps{Size: theme.Small})
			display.Tag(c, statusText, display.TagProps{Color: color, Size: theme.Small})
			if job.Error != "" {
				ui.Text(c, job.Error).FontSize(13).TextColor(tk.ErrorText).Grow(1)
			}
		})
		if len(job.Logs) > 0 {
			start := len(job.Logs) - 4
			if start < 0 {
				start = 0
			}
			for _, line := range job.Logs[start:] {
				ui.Text(c, line).FontSize(12).TextColor(tk.TextSecondary)
			}
		}
	})
}

func decisionColor(decision string) display.TagColor {
	switch decision {
	case "必选":
		return display.TagSuccess
	case "二期":
		return display.TagProcessing
	case "放弃":
		return display.TagInfo
	default:
		return display.TagWarning
	}
}

func factColor(status string) display.TagColor {
	switch status {
	case "已确认":
		return display.TagSuccess
	case "不确定":
		return display.TagWarning
	default:
		return display.TagInfo
	}
}

// playPanel 试玩标签页：开场白 + 对话 + 模拟动作卡。还没开始时是居中的说明。
func playPanel(c *ui.Context) {
	app.mu.Lock()
	intro := app.playIntro
	playErr := app.playErr
	messages := append([]PlayMsg(nil), app.playMessages...)
	streaming := app.playStreaming
	draft := app.playDraft
	draftBefore := draft
	inputGen := app.playInputGen
	feedback := app.playFeedback
	app.mu.Unlock()

	// 还没开场就出错（多半是没配模型）：只给说明和出路，不摆一个发不出去的输入框
	blocked := intro == "" && len(messages) == 0 && !streaming && playErr != ""
	ui.Column(c).Grow(1).Basis(0).Children(func() {
		switch {
		case blocked:
			p := display.EmptyProps{Icon: lucide.CircleAlert(), Title: "现在还不能试玩", Description: playErr}
			if strings.Contains(playErr, "模型设置") {
				p.Action = func() {
					action.Button(c, "去模型设置", action.ButtonProps{Variant: action.Primary}).OnClick(gotoSettings)
				}
			}
			centerEmpty(c, p)
		case intro == "" && len(messages) == 0 && !streaming:
			centerEmpty(c, display.EmptyProps{Icon: lucide.MessagesSquare(), Title: "正在准备试玩…",
				Description: "Agent 会先说明它扮演谁、能帮你做什么，然后你以使用者的身份跟它聊。"})
		default:
			ui.Scroll(c).Grow(1).Basis(0).Children(func() {
				chatColumn(c, func() {
					ui.Column(c).Grow(1).Basis(0).MaxWidth(chatWidth).Padding(24, 0).Gap(14).Children(func() {
						if intro != "" {
							markdown.Markdown(c, intro, markdown.MarkdownProps{Variant: markdown.Chat})
						}
						for _, m := range messages {
							switch m.Role {
							case "user":
								userBubble(c, m.Text)
							case "step":
								stepCard(c, m.Step)
							default:
								markdown.Markdown(c, m.Text, markdown.MarkdownProps{Variant: markdown.Chat})
							}
						}
						if feedback && !streaming {
							ui.Row(c).Gap(6).Children(func() {
								action.Button(c, "这里不对", action.ButtonProps{Size: theme.Small}).OnClick(func() {
									go app.sendPlay("这里不对")
								})
							})
						}
						if playErr != "" {
							errorAlert(c, playErr)
						}
					})
				})
			})
		}
		if blocked {
			return
		}
		if text := chatComposer(c, inputGen, &draft, composerProps{
			Placeholder: "以使用者的身份说一句话…", Busy: streaming, BusyText: "Agent 正在按方案跑工具…",
		}); text != "" {
			draft = ""
			app.mu.Lock()
			app.playInputGen++
			app.mu.Unlock()
			go app.sendPlay(text)
		}
		writeBack(&app.playDraft, draftBefore, draft)
	})
}

// userBubble 用户说的话：靠右的圆角气泡。
func userBubble(c *ui.Context, text string) {
	tk := theme.TokensOf(c)
	ui.Column(c).AlignSelf(ui.End).MaxWidth(560).Padding(10, 14).Radius(16).Background(tk.FillSecondary).Children(func() {
		ui.Text(c, text).FontSize(15)
	})
}

// stepCard 一次模拟动作：动作名 + 输入/结果。
func stepCard(c *ui.Context, step map[string]string) {
	tk := theme.TokensOf(c)
	ui.Column(c).Padding(12, 14).Gap(6).Radius(12).Border(1, tk.BorderSecondary).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, lucide.Wrench()).Width(14).Height(14).TextColor(tk.TextTertiary)
			if step["tool"] != "" {
				ui.Text(c, step["tool"]).FontSize(13).Bold()
			}
			if step["action"] != "" {
				display.Tag(c, step["action"], display.TagProps{Size: theme.Small})
			}
			if step["who"] != "" {
				ui.Text(c, "由 "+step["who"]+" 执行").FontSize(12).TextColor(tk.TextTertiary)
			}
		})
		if step["input"] != "" {
			ui.Text(c, "输入："+step["input"]).FontSize(13).TextColor(tk.TextSecondary)
		}
		if step["result"] != "" {
			ui.Text(c, "结果："+step["result"]).FontSize(13).TextColor(tk.TextSecondary)
		}
	})
}

// auditsView 项目检查页：没检查过时是居中的引导；有结果后左列表 + 右报告。
func auditsView(c *ui.Context) {
	tk := theme.TokensOf(c)
	app.mu.Lock()
	audits := append([]AuditSummary(nil), app.audits...)
	auditsErr := app.auditsErr
	selected := app.auditSelected
	detail := app.auditDetail
	detailErr := app.auditDetailErr
	job := app.auditJob
	busy := app.auditBusy
	status := app.auditStatus
	app.mu.Unlock()
	empty := len(audits) == 0 && auditsErr == ""

	pickButton := func(size theme.Size) {
		action.Button(c, "选文件夹检查", action.ButtonProps{Variant: action.Primary, Size: size}).
			OnClick(func() { go app.pickAndAudit() })
	}

	pageShell(c, func() {
		ui.Row(c).Grow(1).MinWidth(0).Children(func() { pageTitle(c, "项目检查", "看看已有的 AI 项目还差什么") })
		if busy || status != "" {
			ui.Text(c, status).FontSize(12).TextColor(tk.TextTertiary).MaxLines(1)
		}
		if selected != "" && detail != nil {
			action.Button(c, "重新检查", action.ButtonProps{Size: theme.Small}).
				OnClick(func() { go app.rerunAudit(selected) })
		}
		if !empty {
			pickButton(theme.Small)
		}
	}, func() {
		if empty {
			centerEmpty(c, display.EmptyProps{Icon: lucide.FolderSearch(), Title: "检查一个已有的 AI 项目",
				Description: "选一个项目文件夹，引擎会读一遍代码，列出缺失和薄弱的地方，再写出它本该有的 PRD。",
				Action:      func() { pickButton(theme.Middle) }})
			return
		}
		ui.Row(c).Grow(1).Basis(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			// 左：检查结果列表，右侧一条细分隔线
			ui.Column(c).Width(260).Shrink(0).Padding(12, 8).Gap(4).
				BorderWidth(0, 1, 0, 0).BorderColor(tk.BorderSecondary).Children(func() {
				if auditsErr != "" {
					errorAlert(c, auditsErr)
				}
				items := make([]display.ListItem, 0, len(audits))
				for _, a := range audits {
					desc := a.Verdict
					if !a.Finished {
						desc = "检查中…"
					}
					if desc == "" {
						desc = a.Root
					}
					items = append(items, display.ListItem{
						Key: a.Slug, Label: a.Name, Description: desc, Icon: lucide.FolderSearch(),
						Extra: fmt.Sprintf("%.0f%%", a.Completion*100),
					})
				}
				clicked := display.List(c, display.ListProps{Items: items, Active: selected})
				if clicked != "" {
					go app.selectAudit(clicked)
				}
			})
			// 右：报告
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
				if selected == "" {
					centerEmpty(c, display.EmptyProps{Icon: lucide.FileText(), Title: "选一个检查结果",
						Description: "左边选一个项目，这里显示它的差距报告。"})
					return
				}
				readingScroll(c, 16, func() {
					if job != nil && job.Status != "" {
						jobBlock(c, job)
					}
					if detailErr != "" {
						errorAlert(c, detailErr)
						return
					}
					if detail == nil {
						mutedText(c, "读取中…")
						return
					}
					ui.Row(c).Gap(6).AlignItems(ui.Center).Wrap().Children(func() {
						if detail.Verdict != "" {
							display.Tag(c, detail.Verdict, display.TagProps{Color: display.TagInfo, Size: theme.Small})
						}
						display.Tag(c, fmt.Sprintf("缺失 %d", detail.Missing),
							display.TagProps{Color: display.TagError, Size: theme.Small})
						display.Tag(c, fmt.Sprintf("薄弱 %d", detail.Weak),
							display.TagProps{Color: display.TagWarning, Size: theme.Small})
						if detail.Finished {
							ui.Text(c, fmt.Sprintf("读过 %d 个文件 · %d 次调用", detail.FilesSeen, detail.Calls)).
								FontSize(12).TextColor(tk.TextTertiary)
						}
					})
					if detail.Report != "" {
						markdown.Markdown(c, detail.Report, markdown.MarkdownProps{})
					}
					if detail.SuggestedPRD != "" {
						sectionCard(c, "本该有的 PRD", 0, func() {
							markdown.Markdown(c, detail.SuggestedPRD, markdown.MarkdownProps{})
						})
					}
				})
			})
		})
	})
}

// newProductView 对话式新建产品：追问流 + 确认卡片 + 快捷回复。
func newProductView(c *ui.Context) {
	app.mu.Lock()
	messages := append([]ChatMsg(nil), app.chatMessages...)
	streaming := app.chatStreaming
	chatErr := app.chatError
	confirm := app.chatConfirm
	suggest := append([]string(nil), app.chatSuggest...)
	draft := app.chatDraft
	draftBefore := draft
	inputGen := app.chatInputGen
	app.mu.Unlock()
	send := func(text string) {
		app.mu.Lock()
		app.chatInputGen++
		app.mu.Unlock()
		go app.sendChat(text)
	}

	pageShell(c, func() {
		pageTitle(c, "新建产品", "一次问一个问题，问清楚后整理成需求草稿")
	}, func() {
		if len(messages) == 0 && !streaming {
			newProductWelcome(c, send)
		} else {
			ui.Scroll(c).Grow(1).Basis(0).Children(func() {
				chatColumn(c, func() {
					ui.Column(c).Grow(1).Basis(0).MaxWidth(chatWidth).Padding(24, 0).Gap(14).Children(func() {
						for _, m := range messages {
							if m.Role == "user" {
								userBubble(c, m.Text)
							} else {
								markdown.Markdown(c, m.Text, markdown.MarkdownProps{Variant: markdown.Chat})
							}
						}
						if confirm != nil {
							confirmCard(c, confirm)
						}
						if len(suggest) > 0 && !streaming {
							ui.Row(c).Gap(8).Wrap().Children(func() {
								for _, s := range suggest {
									action.Button(c, s, action.ButtonProps{Size: theme.Small}).OnClick(func() { send(s) })
								}
							})
						}
						if chatErr != "" {
							errorAlert(c, chatErr)
						}
					})
				})
			})
		}
		if text := chatComposer(c, inputGen, &draft, composerProps{
			Placeholder: "描述你想做的 AI 产品，比如：帮社区面包店回复顾客微信的 AI 客服",
			Busy:        streaming, BusyText: "引擎正在思考…",
		}); text != "" {
			draft = ""
			send(text)
		}
		writeBack(&app.chatDraft, draftBefore, draft)
	})
}

// newProductWelcome 新建产品的空状态：居中的欢迎语和几个可以直接点的例子。
func newProductWelcome(c *ui.Context, send func(string)) {
	tk := theme.TokensOf(c)
	examples := []string{
		"帮社区面包店回复顾客微信的 AI 客服，能查今天的面包、接预订",
		"帮我妈记账的 AI，她发一句「买菜 35」就记下来，月底给她总结",
		"每天早上自动整理我的工作邮件，挑出要回的并起草回复",
	}
	ui.Column(c).Grow(1).Basis(0).Justify(ui.Center).Children(func() {
		chatColumn(c, func() {
			ui.Column(c).Grow(1).Basis(0).MaxWidth(chatWidth).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, lucide.Sparkles()).Width(28).Height(28).TextColor(tk.TextSecondary)
				ui.Text(c, "说说你想做的 AI 产品").FontSize(24).Bold()
				ui.Text(c, "我会一次问你一个问题，问清楚后整理成需求草稿给你确认。说不清的地方直接说「不知道」就行。").
					FontSize(14).TextColor(tk.TextSecondary).TextAlign(ui.Center)
				ui.Column(c).Gap(8).Padding(14, 0, 0, 0).AlignSelf(ui.Stretch).Children(func() {
					for _, ex := range examples {
						row := ui.ButtonBase(c).Padding(10, 14).Radius(12).Border(1, tk.BorderSecondary).
							AlignItems(ui.Center).Justify(ui.Start).Gap(8).Row().Cursor(ui.CursorPointer)
						if row.Hovered() {
							row.Background(tk.FillQuaternary)
						}
						row.Children(func() {
							ui.Icon(c, lucide.CornerDownLeft()).Width(14).Height(14).TextColor(tk.TextTertiary)
							ui.Text(c, ex).FontSize(14).TextColor(tk.TextSecondary)
						})
						row.OnClick(func() { send(ex) })
					}
				})
			})
		})
	})
}

// confirmCard 引擎给出草稿后的确认卡片。
func confirmCard(c *ui.Context, confirm *ConfirmCard) {
	tk := theme.TokensOf(c)
	ui.Column(c).Padding(16, 18).Gap(8).Radius(12).Border(1, tk.Border).Children(func() {
		ui.Text(c, "草稿好了，确认要创建吗？").FontSize(14).Bold()
		ui.Text(c, fmt.Sprintf("产品名：%s", confirm.Name)).FontSize(14)
		ui.Text(c, confirm.Idea).FontSize(13).TextColor(tk.TextSecondary)
		ui.Row(c).Gap(8).Padding(4, 0, 0, 0).Children(func() {
			action.Button(c, "确认创建", action.ButtonProps{Variant: action.Primary}).OnClick(func() {
				go app.confirmProduct()
			})
			action.Button(c, "再改改", action.ButtonProps{}).OnClick(func() {
				app.mu.Lock()
				app.chatConfirm = nil
				app.mu.Unlock()
				app.invalidate()
			})
		})
	})
}

// settingsView 模型设置：居中的一张表单卡片，每项带一句说明，保存按钮在右下。
func settingsView(c *ui.Context) {
	tk := theme.TokensOf(c)
	app.mu.Lock()
	settings := app.settings
	settingsErr := app.settingsErr
	provider := app.setProvider
	model := app.setModel
	key := app.setKey
	baseURL := app.setBaseURL
	status := app.setStatus
	app.mu.Unlock()
	providerBefore, modelBefore, keyBefore, baseURLBefore := provider, model, key, baseURL

	providers := make([]field.SelectOption, 0, 8)
	customBase := false
	if raw, ok := settings["providers"].([]any); ok {
		for _, r := range raw {
			if m, ok := r.(map[string]any); ok {
				pk := toString(m["key"])
				providers = append(providers, field.SelectOption{Value: pk, Label: toString(m["name"])})
				if pk == provider && m["custom_base"] == true {
					customBase = true
				}
			}
		}
	}

	pageShell(c, func() {
		pageTitle(c, "模型设置", "")
	}, func() {
		ui.Scroll(c).Grow(1).Basis(0).Children(func() {
			chatColumn(c, func() {
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).MaxWidth(560).Padding(28, 0, 40, 0).Gap(16).Children(func() {
					ui.Column(c).Gap(6).Children(func() {
						ui.Text(c, "选择模型").FontSize(22).Bold()
						ui.Text(c, "新建产品时的追问、生成方案和试玩，都用这里配置的模型。").FontSize(14).TextColor(tk.TextSecondary)
					})
					if settingsErr != "" {
						errorAlert(c, settingsErr)
						return
					}
					if settings == nil {
						mutedText(c, "读取中…")
						return
					}
					ui.Column(c).Padding(20).Gap(18).Radius(12).Border(1, tk.BorderSecondary).Children(func() {
						formField(c, "模型服务商", "选一个你有 API Key 的服务商。", func() {
							field.Select(c, &provider, field.SelectProps{Options: providers, Placeholder: "选择服务商", Size: theme.Middle})
							writeBack(&app.setProvider, providerBefore, provider)
						})
						formField(c, "模型名", "不填就用服务商推荐的默认模型。", func() {
							field.Input(c, &model, field.InputProps{Placeholder: "留空用默认模型", Size: theme.Middle})
							writeBack(&app.setModel, modelBefore, model)
						})
						formField(c, "API Key", "已保存的 Key 不会显示出来，重新粘贴才会覆盖。", func() {
							field.InputPassword(c, &key, field.InputProps{Placeholder: "粘贴 API Key", Size: theme.Middle})
							writeBack(&app.setKey, keyBefore, key)
						})
						if customBase {
							formField(c, "接口地址", "这个服务商需要自己填接口地址，一般以 /v1 结尾。", func() {
								field.Input(c, &baseURL, field.InputProps{Placeholder: "https://…/v1", Size: theme.Middle})
								writeBack(&app.setBaseURL, baseURLBefore, baseURL)
							})
						}
					})
					ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
						color := tk.TextSecondary
						switch {
						case strings.HasPrefix(status, "保存失败"):
							color = tk.ErrorText
						case strings.HasPrefix(status, "已保存"):
							color = tk.SuccessText
						}
						ui.Text(c, status).FontSize(13).TextColor(color).Grow(1).Basis(0).MinWidth(0)
						action.Button(c, "保存", action.ButtonProps{Variant: action.Primary}).OnClick(func() {
							app.mu.Lock()
							app.setProvider, app.setModel, app.setKey, app.setBaseURL = provider, model, key, baseURL
							app.mu.Unlock()
							go app.saveSettings()
						})
					})
				})
			})
		})
	})
}

// formField 表单里的一项：标签、控件、一句灰色说明。
func formField(c *ui.Context, label, help string, control func()) {
	tk := theme.TokensOf(c)
	ui.Column(c).Gap(6).Children(func() {
		ui.Text(c, label).FontSize(13).FontWeight(500)
		control()
		if help != "" {
			ui.Text(c, help).FontSize(12).TextColor(tk.TextTertiary)
		}
	})
}

// revealProduct 调引擎的 reveal 接口，让 Finder 打开工作区目录。
func revealProduct(base, slug string) {
	_ = postJSON(base, "/api/products/"+url.PathEscape(slug)+"/reveal", "{}")
}

// writeBack 把输入框这一帧改过的值写回界面状态。
// 输入框绑的是每帧从 app 复制出来的局部变量，不写回的话打的字只留在控件里，
// 发送 / 保存时读到的还是旧值（空串）。只在这一帧真改了才写，免得盖掉后台刚做的清空。
func writeBack(dst *string, before, after string) {
	if after == before {
		return
	}
	app.mu.Lock()
	*dst = after
	app.mu.Unlock()
}
