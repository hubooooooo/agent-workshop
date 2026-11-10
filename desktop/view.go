package main

import (
	"fmt"
	"net/url"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/markdown"
	"github.com/lobehub/lobe-mygo/pkg/nav"
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
			app.mu.Lock()
			app.page = PageSettings
			app.mu.Unlock()
			go app.loadSettings()
		}

		// 产品列表
		items := make([]display.ListItem, 0, len(products))
		for _, p := range products {
			desc := fmt.Sprintf("V%d · 待确认 %d", p.Version, p.Unknowns)
			if !p.HasPlan {
				desc = fmt.Sprintf("V%d · 还没有方案", p.Version)
			}
			items = append(items, display.ListItem{
				Key:         p.Slug,
				Label:       p.Name,
				Description: desc,
				Icon:        lucide.Boxes(),
				Extra:       fmt.Sprintf("V%d", p.Version),
			})
		}
		if len(items) == 0 {
			items = append(items, display.ListItem{
				Key: "~empty", Label: "还没有产品", Description: "去「新建产品」聊一个出来",
				Icon: lucide.Sparkles(), Disabled: true,
			})
		}
		clicked := display.List(c, display.ListProps{Items: items, Active: selected})
		if clicked != "" && clicked != "~empty" {
			go app.selectProduct(clicked)
		}

		// 底部：引擎状态
		status, color := "引擎启动中…", tk.TextSecondary
		switch {
		case engineErr != "":
			status, color = "⚠ "+engineErr, tk.ErrorText
		case up && len(products) > 0:
			status = fmt.Sprintf("引擎已连接 · %d 个产品", len(products))
		case up:
			status = "引擎已连接"
		}
		ui.Text(c, status).FontSize(12).TextColor(color).Padding(0, 4, 0, 0)
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

// productView 产品详情：PRD 与结构化方案两个标签页。
func productView(c *ui.Context) {
	tk := theme.TokensOf(c)
	app.mu.Lock()
	selected := app.selected
	prdText := app.prdText
	prdVer := app.prdVer
	prdErr := app.prdErr
	loading := app.loading
	tab := app.productTab
	name := ""
	for _, p := range app.products {
		if p.Slug == selected {
			name = p.Name
		}
	}
	app.mu.Unlock()

	ui.Column(c).Grow(1).Basis(0).MinWidth(0).Margin(8, 8, 8, 0).
		Radius(tk.BorderRadiusLG).Background(tk.BgContainer).Border(1, tk.Border).Clip().
		Children(func() {
			ui.Row(c).Height(46).Shrink(0).Padding(16, 8).AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
				title := "选一个产品"
				if selected != "" {
					title = name
					if prdVer > 0 {
						title = fmt.Sprintf("%s · V%d", name, prdVer)
					}
				}
				ui.Text(c, title).FontSize(15).Bold()
				if selected != "" {
					ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
						action.ActionIcon(c, lucide.PackageOpen(), action.ActionIconProps{Title: "下载交接包"}).OnClick(func() {
							go app.downloadHandoffZip(selected)
						})
						action.ActionIcon(c, lucide.FolderOpen(), action.ActionIconProps{Title: "在 Finder 里打开工作区"}).OnClick(func() {
							go revealProduct(app.engineURL, selected)
						})
					})
				}
			})
			if selected == "" {
				ui.Scroll(c).Grow(1).Basis(0).Children(func() {
					ui.Column(c).Padding(16, 24).Children(func() {
						ui.Text(c, "左边选一个产品，这里显示它的 PRD 和方案。").TextColor(tk.TextSecondary)
					})
				})
			} else {
				nav.Tabs(c, &tab, nav.TabsProps{Items: []nav.TabItem{
					{Label: "PRD"}, {Label: "方案"}, {Label: "试玩"},
				}, Size: theme.Small}, func(i int) {
					switch i {
					case 0:
						prdPanel(c, selected, loading, prdErr, prdText)
					case 1:
						planPanel(c)
					default:
						playPanel(c, selected)
					}
				}).Grow(1)
			}
			app.mu.Lock()
			if tab != app.productTab && tab == 2 {
				go app.startPlay(selected)
			}
			app.productTab = tab
			app.mu.Unlock()
		})
}

// prdPanel PRD 标签页内容。
func prdPanel(c *ui.Context, selected string, loading bool, prdErr, prdText string) {
	tk := theme.TokensOf(c)
	ui.Scroll(c).Grow(1).Basis(0).Children(func() {
		ui.Column(c).Padding(16, 24).Gap(8).Children(func() {
			switch {
			case selected == "":
				ui.Text(c, "左边选一个产品。").TextColor(tk.TextSecondary)
			case loading:
				ui.Text(c, "读取中…").TextColor(tk.TextSecondary)
			case prdErr != "":
				ui.Text(c, "⚠ "+prdErr).TextColor(tk.ErrorText)
			default:
				markdown.Markdown(c, prdText, markdown.MarkdownProps{})
			}
		})
	})
}

// planPanel 方案标签页：任务进度、待确认清单、组件选型、七层架构等。
func planPanel(c *ui.Context) {
	tk := theme.TokensOf(c)
	app.mu.Lock()
	plan := app.plan
	planErr := app.planErr
	job := app.job
	app.mu.Unlock()

	ui.Scroll(c).Grow(1).Basis(0).Children(func() {
		ui.Column(c).Padding(16, 24).Gap(18).MaxWidth(760).Children(func() {
			if planErr != "" {
				ui.Text(c, "⚠ "+planErr).TextColor(tk.ErrorText)
				return
			}
			if plan == nil {
				ui.Text(c, "读取方案中…").TextColor(tk.TextSecondary)
				return
			}

			// 任务进度
			if job != nil && job.Status != "" {
				jobBlock(c, job)
			}

			// 标题与定位
			if plan.Title != "" {
				ui.Text(c, plan.Title).FontSize(18).Bold()
			}
			if plan.OneLiner != "" {
				ui.Text(c, plan.OneLiner).FontSize(14).TextColor(tk.TextSecondary)
			}
			if plan.Runner != "" || plan.Model != "" {
				ui.Row(c).Gap(6).Children(func() {
					if plan.Runner != "" {
						display.Tag(c, plan.Runner, display.TagProps{})
					}
					if plan.Model != "" {
						display.Tag(c, plan.Model, display.TagProps{})
					}
				})
			}

			// 待确认清单
			if len(plan.Unknowns) > 0 {
				sectionTitle(c, fmt.Sprintf("待确认清单（%d）", len(plan.Unknowns)))
				for i, q := range plan.Unknowns {
					ui.Row(c).Gap(8).AlignItems(ui.Start).Children(func() {
						ui.Text(c, fmt.Sprintf("%d.", i+1)).FontSize(14).Bold().TextColor(tk.PrimaryText)
						ui.Text(c, q).FontSize(14).Grow(1)
					})
				}
			}

			// 四项事实
			if len(plan.Facts) > 0 {
				sectionTitle(c, "已确认的事实")
				for _, f := range plan.Facts {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Text(c, f.Label).FontSize(13).Bold().Width(90)
						ui.Text(c, f.Value).FontSize(13).Grow(1)
						display.Tag(c, f.Status, display.TagProps{Color: factColor(f.Status), Size: theme.Small})
					})
				}
			}

			// 组件选型
			if len(plan.Components) > 0 {
				sectionTitle(c, "组件选型")
				for _, comp := range plan.Components {
					ui.Row(c).Gap(10).AlignItems(ui.Start).Children(func() {
						display.Tag(c, comp.Decision, display.TagProps{Color: decisionColor(comp.Decision), Size: theme.Small})
						ui.Column(c).Grow(1).Gap(2).Children(func() {
							ui.Text(c, comp.OneLiner).FontSize(14).Bold()
							if comp.Reason != "" {
								ui.Text(c, comp.Reason).FontSize(13).TextColor(tk.TextSecondary)
							}
						})
					})
				}
			}

			// 七层架构
			if len(plan.Layers) > 0 {
				sectionTitle(c, "七层架构")
				for _, layer := range plan.Layers {
					ui.Column(c).Gap(3).Children(func() {
						ui.Text(c, fmt.Sprintf("%d · %s", layer.Num, layer.Name)).FontSize(14).Bold()
						if layer.Detail != "" {
							ui.Text(c, layer.Detail).FontSize(13).TextColor(tk.TextSecondary)
						}
						if layer.Question != "" {
							ui.Text(c, "待确认："+layer.Question).FontSize(13).TextColor(tk.ErrorText)
						}
					})
				}
			}

			// 工具清单
			if len(plan.ToolList) > 0 {
				sectionTitle(c, "工具清单")
				for _, tool := range plan.ToolList {
					ui.Text(c, "· "+tool).FontSize(13)
				}
			}

			// 降级与安全
			if len(plan.SecurityAndDegradation) > 0 {
				sectionTitle(c, "降级与安全")
				for _, item := range plan.SecurityAndDegradation {
					ui.Text(c, "· "+item).FontSize(13)
				}
			}

			// 测试设计
			if len(plan.Testing) > 0 {
				sectionTitle(c, "测试设计")
				for _, item := range plan.Testing {
					ui.Text(c, "· "+item).FontSize(13)
				}
			}

			// 系统提示词草案
			if plan.SystemPrompt != "" {
				sectionTitle(c, "系统提示词草案")
				ui.Column(c).Padding(12).Radius(8).Background(tk.FillQuaternary).Children(func() {
					ui.Text(c, plan.SystemPrompt).FontSize(12).TextColor(tk.TextSecondary)
				})
			}

			// 独立审稿
			if plan.Review != nil {
				ok, _ := plan.Review["ok"].(bool)
				sectionTitle(c, "独立审稿")
				if ok {
					display.Tag(c, "审稿通过", display.TagProps{Color: display.TagSuccess})
				} else {
					display.Tag(c, "审稿未通过", display.TagProps{Color: display.TagWarning})
				}
			}
		})
	})
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
	ui.Column(c).Padding(12).Gap(6).Radius(8).Background(tk.FillQuaternary).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			display.Tag(c, "推导任务", display.TagProps{})
			display.Tag(c, statusText, display.TagProps{Color: color})
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

func sectionTitle(c *ui.Context, title string) {
	ui.Text(c, title).FontSize(15).Bold()
}

func decisionColor(decision string) display.TagColor {
	switch decision {
	case "必选":
		return display.TagSuccess
	case "二期":
		return display.TagProcessing
	case "放弃":
		return display.TagInfo
	case "待确认":
		return display.TagWarning
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

// playPanel 试玩标签页：开场白 + 对话 + 模拟动作卡。
func playPanel(c *ui.Context, slug string) {
	tk := theme.TokensOf(c)
	app.mu.Lock()
	intro := app.playIntro
	playErr := app.playErr
	messages := append([]PlayMsg(nil), app.playMessages...)
	streaming := app.playStreaming
	draft := app.playDraft
	feedback := app.playFeedback
	handoffBusy := app.handoffBusy
	handoffStatus := app.handoffStatus
	app.mu.Unlock()

	ui.Column(c).Grow(1).Basis(0).Children(func() {
		if handoffBusy || handoffStatus != "" {
			ui.Row(c).Shrink(0).Padding(2, 8).AlignItems(ui.Center).Gap(8).Children(func() {
				if handoffBusy {
					ui.Text(c, "正在打包交接包…").FontSize(12).TextColor(tk.TextSecondary)
				} else if handoffStatus != "" {
					ui.Text(c, handoffStatus).FontSize(12).TextColor(tk.TextSecondary)
				}
			})
		}
		ui.Scroll(c).Grow(1).Basis(0).Children(func() {
			ui.Column(c).Padding(16, 24).Gap(12).MaxWidth(760).Children(func() {
				if intro != "" {
					markdown.Markdown(c, intro, markdown.MarkdownProps{Variant: markdown.Chat})
				}
				for _, m := range messages {
					switch m.Role {
					case "user":
						ui.Column(c).AlignSelf(ui.End).MaxWidth(520).
							Padding(8, 12).Radius(12).Background(tk.PrimaryBg).Children(func() {
							ui.Text(c, m.Text).FontSize(14)
						})
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
					ui.Text(c, "⚠ "+playErr).FontSize(13).TextColor(tk.ErrorText)
				}
				if streaming {
					ui.Text(c, "Agent 正在按方案跑工具…").FontSize(12).TextColor(tk.TextSecondary)
				}
			})
		})
		ui.Row(c).Shrink(0).Padding(12, 16).Gap(8).AlignItems(ui.Center).Children(func() {
			field.Input(c, &draft, field.InputProps{Placeholder: "以使用者的身份说一句话…", Size: theme.Middle}).Grow(1).
				OnSubmit(func() { go app.sendPlay(draft) })
			action.Button(c, "发送", action.ButtonProps{Variant: action.Primary, Size: theme.Middle}).
				OnClick(func() { go app.sendPlay(draft) })
		})
	})
}

// stepCard 一次模拟动作：动作名 + 输入/结果。
func stepCard(c *ui.Context, step map[string]string) {
	tk := theme.TokensOf(c)
	ui.Column(c).Padding(10, 12).Gap(4).Radius(8).Background(tk.FillQuaternary).Border(1, tk.BorderSecondary).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			display.Tag(c, "动作", display.TagProps{Color: display.TagProcessing, Size: theme.Small})
			if step["tool"] != "" {
				ui.Text(c, step["tool"]).FontSize(13).Bold()
			}
			if step["action"] != "" {
				display.Tag(c, step["action"], display.TagProps{Size: theme.Small})
			}
			if step["who"] != "" {
				ui.Text(c, "由 "+step["who"]+" 执行").FontSize(12).TextColor(tk.TextSecondary)
			}
		})
		if step["input"] != "" {
			ui.Text(c, "输入："+step["input"]).FontSize(12).TextColor(tk.TextSecondary)
		}
		if step["result"] != "" {
			ui.Text(c, "结果："+step["result"]).FontSize(12).TextColor(tk.TextSecondary)
		}
	})
}

// auditsView 项目检查页：左列表 + 右报告。
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

	ui.Column(c).Grow(1).Basis(0).MinWidth(0).Margin(8, 8, 8, 0).
		Radius(tk.BorderRadiusLG).Background(tk.BgContainer).Border(1, tk.Border).Clip().
		Children(func() {
			ui.Row(c).Height(46).Shrink(0).Padding(16, 8).AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "项目检查").FontSize(15).Bold()
					action.Button(c, "选文件夹检查", action.ButtonProps{Variant: action.Primary, Size: theme.Small}).
						OnClick(func() { go app.pickAndAudit() })
					if selected != "" && detail != nil {
						action.Button(c, "重新检查", action.ButtonProps{Size: theme.Small}).
							OnClick(func() { go app.rerunAudit(selected) })
					}
					if busy || status != "" {
						ui.Text(c, status).FontSize(12).TextColor(tk.TextSecondary)
					}
				})
			})
			ui.Row(c).Grow(1).Basis(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
				// 左：检查结果列表
				ui.Column(c).Width(248).Shrink(0).Padding(12, 8).Gap(4).Children(func() {
					if auditsErr != "" {
						ui.Text(c, "⚠ "+auditsErr).FontSize(12).TextColor(tk.ErrorText)
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
					if len(items) == 0 {
						ui.Text(c, "还没有检查过项目。点上面「选文件夹检查」。").FontSize(12).TextColor(tk.TextSecondary)
					}
					clicked := display.List(c, display.ListProps{Items: items, Active: selected})
					if clicked != "" {
						go app.selectAudit(clicked)
					}
				})
				// 右：报告
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
					if selected == "" {
						ui.Column(c).Grow(1).Children(func() {
							ui.Text(c, "选一个检查结果看差距报告。").TextColor(tk.TextSecondary).Padding(16, 12)
						})
						return
					}
					if job != nil && job.Status != "" {
						ui.Column(c).Shrink(0).Padding(8, 12, 0, 12).Children(func() {
							jobBlock(c, job)
						})
					}
					if detailErr != "" {
						ui.Text(c, "⚠ "+detailErr).TextColor(tk.ErrorText).Padding(16, 12)
						return
					}
					if detail == nil {
						ui.Text(c, "读取中…").TextColor(tk.TextSecondary).Padding(16, 12)
						return
					}
					// 摘要行
					ui.Row(c).Shrink(0).Padding(4, 12).Gap(6).AlignItems(ui.Center).Children(func() {
						if detail.Verdict != "" {
							display.Tag(c, detail.Verdict, display.TagProps{Color: display.TagInfo, Size: theme.Small})
						}
						display.Tag(c, fmt.Sprintf("缺失 %d", detail.Missing),
							display.TagProps{Color: display.TagError, Size: theme.Small})
						display.Tag(c, fmt.Sprintf("薄弱 %d", detail.Weak),
							display.TagProps{Color: display.TagWarning, Size: theme.Small})
						if detail.Finished {
							ui.Text(c, fmt.Sprintf("读过 %d 个文件 · %d 次调用", detail.FilesSeen, detail.Calls)).
								FontSize(12).TextColor(tk.TextSecondary)
						}
					})
					ui.Scroll(c).Grow(1).Basis(0).Children(func() {
						ui.Column(c).Padding(16, 24, 16, 16).Gap(18).MaxWidth(760).Children(func() {
							if detail.Report != "" {
								markdown.Markdown(c, detail.Report, markdown.MarkdownProps{})
							}
							if detail.SuggestedPRD != "" {
								sectionTitle(c, "本该有的 PRD")
								markdown.Markdown(c, detail.SuggestedPRD, markdown.MarkdownProps{})
							}
						})
					})
				})
			})
		})
}

// newProductView 对话式新建产品：追问流 + 确认卡片 + 快捷回复。
func newProductView(c *ui.Context) {
	tk := theme.TokensOf(c)
	app.mu.Lock()
	messages := append([]ChatMsg(nil), app.chatMessages...)
	streaming := app.chatStreaming
	chatErr := app.chatError
	confirm := app.chatConfirm
	suggest := append([]string(nil), app.chatSuggest...)
	draft := app.chatDraft
	app.mu.Unlock()

	ui.Column(c).Grow(1).Basis(0).MinWidth(0).Margin(8, 8, 8, 0).
		Radius(tk.BorderRadiusLG).Background(tk.BgContainer).Border(1, tk.Border).Clip().
		Children(func() {
			ui.Row(c).Height(46).Shrink(0).Padding(16, 8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "新建产品").FontSize(15).Bold()
			})
			ui.Scroll(c).Grow(1).Basis(0).Children(func() {
				ui.Column(c).Padding(16, 24).Gap(12).Children(func() {
					if len(messages) == 0 {
						ui.Text(c, "说说你的想法，引擎会追问到说清楚为止（最多 6 轮）。").TextColor(tk.TextSecondary)
					}
					for _, m := range messages {
						if m.Role == "user" {
							ui.Column(c).AlignSelf(ui.End).MaxWidth(520).
								Padding(8, 12).Radius(12).Background(tk.PrimaryBg).Children(func() {
								ui.Text(c, m.Text).FontSize(14)
							})
						} else {
							ui.Column(c).Grow(1).MaxWidth(640).Children(func() {
								markdown.Markdown(c, m.Text, markdown.MarkdownProps{Variant: markdown.Chat})
							})
						}
					}
					if confirm != nil {
						confirmCard(c, confirm)
					}
					if len(suggest) > 0 && !streaming {
						ui.Row(c).Gap(6).Children(func() {
							for _, s := range suggest {
								action.Button(c, s, action.ButtonProps{Size: theme.Small}).OnClick(func() {
									go app.sendChat(s)
								})
							}
						})
					}
					if chatErr != "" {
						ui.Text(c, "⚠ "+chatErr).FontSize(13).TextColor(tk.ErrorText)
					}
				})
			})
			// 输入区
			ui.Row(c).Shrink(0).Padding(12, 16).Gap(8).AlignItems(ui.Center).Children(func() {
				field.Input(c, &draft, field.InputProps{Placeholder: "描述你的产品点子…", Size: theme.Middle}).Grow(1).
					OnSubmit(func() { go app.sendChat(draft) })
				action.Button(c, "发送", action.ButtonProps{Variant: action.Primary, Size: theme.Middle}).
					OnClick(func() { go app.sendChat(draft) })
			})
			if streaming {
				ui.Text(c, "引擎正在思考…").FontSize(12).TextColor(tk.TextSecondary).Padding(0, 0, 8, 16)
			}
		})
}

// confirmCard 引擎给出草稿后的确认卡片。
func confirmCard(c *ui.Context, confirm *ConfirmCard) {
	tk := theme.TokensOf(c)
	ui.Column(c).Padding(16).Gap(8).Radius(tk.BorderRadiusLG).Background(tk.FillQuaternary).Border(1, tk.BorderSecondary).Children(func() {
		ui.Text(c, "草稿好了，确认要创建吗？").FontSize(14).Bold()
		ui.Text(c, fmt.Sprintf("产品名：%s", confirm.Name)).FontSize(13)
		ui.Text(c, confirm.Idea).FontSize(13).TextColor(tk.TextSecondary)
		ui.Row(c).Gap(8).Children(func() {
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

// settingsView 模型设置：服务商 / 模型名 / Key / 接口地址。
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

	ui.Column(c).Grow(1).Basis(0).MinWidth(0).Margin(8, 8, 8, 0).
		Radius(tk.BorderRadiusLG).Background(tk.BgContainer).Border(1, tk.Border).Clip().
		Children(func() {
			ui.Row(c).Height(46).Shrink(0).Padding(16, 8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "模型设置").FontSize(15).Bold()
			})
			ui.Scroll(c).Grow(1).Basis(0).Children(func() {
				ui.Column(c).Padding(16, 24).Gap(14).MaxWidth(560).Children(func() {
					if settingsErr != "" {
						ui.Text(c, "⚠ "+settingsErr).TextColor(tk.ErrorText)
						return
					}
					if settings == nil {
						ui.Text(c, "读取中…").TextColor(tk.TextSecondary)
						return
					}
					ui.Column(c).Gap(6).Children(func() {
						ui.Text(c, "模型服务商").FontSize(13).Bold()
						field.Select(c, &provider, field.SelectProps{Options: providers, Size: theme.Middle})
					})
					ui.Column(c).Gap(6).Children(func() {
						ui.Text(c, "模型名").FontSize(13).Bold()
						field.Input(c, &model, field.InputProps{Placeholder: "留空用服务商默认模型", Size: theme.Middle})
					})
					ui.Column(c).Gap(6).Children(func() {
						ui.Text(c, "API Key").FontSize(13).Bold()
						field.InputPassword(c, &key, field.InputProps{Placeholder: "重新粘贴才覆盖已保存的 Key", Size: theme.Middle})
					})
					if customBase {
						ui.Column(c).Gap(6).Children(func() {
							ui.Text(c, "接口地址").FontSize(13).Bold()
							field.Input(c, &baseURL, field.InputProps{Placeholder: "https://…/v1", Size: theme.Middle})
						})
					}
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						action.Button(c, "保存", action.ButtonProps{Variant: action.Primary}).OnClick(func() {
							app.mu.Lock()
							app.setProvider, app.setModel, app.setKey, app.setBaseURL = provider, model, key, baseURL
							app.mu.Unlock()
							go app.saveSettings()
						})
						if status != "" {
							ui.Text(c, status).FontSize(13).TextColor(tk.TextSecondary)
						}
					})
				})
			})
		})
}

// revealProduct 调引擎的 reveal 接口，让 Finder 打开工作区目录。
func revealProduct(base, slug string) {
	_ = postJSON(base, "/api/products/"+url.PathEscape(slug)+"/reveal", "{}")
}
