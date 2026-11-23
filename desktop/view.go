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
	engineStatus := app.engineStatus
	up := app.engineURL != ""
	cfgLoaded, cfgMock, cfgConfigured, cfgModel := app.cfgLoaded, app.cfgMock, app.cfgConfigured, app.cfgModel
	app.mu.Unlock()
	settingsDesc := ""
	switch {
	case !cfgLoaded:
	case cfgMock:
		settingsDesc = "离线演示"
	case cfgConfigured && cfgModel != "":
		settingsDesc = cfgModel
	case cfgConfigured:
		settingsDesc = "已配置模型"
	default:
		settingsDesc = "未配置模型"
	}

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
			{Key: "settings", Label: "模型设置", Icon: lucide.SlidersHorizontal(), Description: settingsDesc},
		}, Active: navKeyOf(page, selected)})
		switch navClicked {
		case "products":
			app.mu.Lock()
			app.page = PageProducts
			app.selected = "" // 回到「我的产品」列表
			if app.pollStop != nil {
				close(app.pollStop)
				app.pollStop = nil
			}
			app.mu.Unlock()
			go app.refreshProducts()
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
				} else if p.Unknowns == 0 {
					desc = fmt.Sprintf("V%d · 方案已生成", p.Version)
				}
				items = append(items, display.ListItem{
					Key: p.Slug, Label: p.Name, Description: desc, Icon: lucide.Boxes(),
				})
			}
			if len(items) == 0 {
				ui.Text(c, "还没有产品，去「新建产品」聊一个出来。").FontSize(12).TextColor(tk.TextTertiary).Padding(4, 8)
				return
			}
			active := selected
			if page != PageProducts {
				active = ""
			}
			clicked := display.List(c, display.ListProps{Items: items, Active: active})
			if clicked != "" {
				go app.selectProduct(clicked)
			}
		})

		// 底部：引擎状态（小圆点 + 一行字）
		status, dot, color := "引擎启动中…", tk.Warning, tk.TextTertiary
		if engineStatus != "" {
			status = engineStatus
		}
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

func navKeyOf(page Page, selected string) string {
	switch page {
	case PageNew:
		return "new"
	case PageSettings:
		return "settings"
	case PageAudits:
		return "audits"
	default:
		if selected != "" {
			return "" // 选中某个产品时，高亮的是下面产品列表里的那一项
		}
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

// userBubble 用户说的话：靠右的圆角气泡。
func userBubble(c *ui.Context, text string) {
	tk := theme.TokensOf(c)
	ui.Column(c).AlignSelf(ui.End).MaxWidth(560).Padding(10, 14).Radius(16).Background(tk.FillSecondary).Children(func() {
		ui.Text(c, text).FontSize(15)
	})
}

// stepCard 一次模拟动作，和网页版的三种卡片一致：调用工具、要先问人、按规则拦下。
func stepCard(c *ui.Context, step map[string]string) {
	tk := theme.TokensOf(c)
	switch step["kind"] {
	case "ask_owner":
		who := step["who"]
		if who == "" {
			who = "人"
		}
		ui.Row(c).Padding(10, 14).Gap(8).Radius(12).Background(tk.WarningBg).AlignItems(ui.Start).Children(func() {
			ui.Icon(c, lucide.Hand()).Width(14).Height(14).TextColor(tk.WarningText)
			ui.Text(c, "要先问"+who+"："+step["action"]).FontSize(13).TextColor(tk.WarningText).Grow(1).Basis(0).MinWidth(0)
		})
		return
	case "blocked":
		ui.Row(c).Padding(10, 14).Gap(8).Radius(12).Background(tk.ErrorBg).AlignItems(ui.Start).Children(func() {
			ui.Icon(c, lucide.Ban()).Width(14).Height(14).TextColor(tk.ErrorText)
			ui.Text(c, "按规则拦下："+step["rule"]).FontSize(13).TextColor(tk.ErrorText).Grow(1).Basis(0).MinWidth(0)
		})
		return
	}
	ui.Column(c).Padding(10, 14).Gap(4).Radius(12).Border(1, tk.BorderSecondary).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, lucide.Wrench()).Width(14).Height(14).TextColor(tk.TextTertiary)
			ui.Text(c, "模拟调用工具").FontSize(13).TextColor(tk.TextSecondary)
			ui.Text(c, step["tool"]).Font("monospace").FontSize(13).FontWeight(600)
		})
		if step["input"] != "" {
			ui.Text(c, "输入："+step["input"]).FontSize(13).TextColor(tk.TextSecondary)
		}
		if step["result"] != "" {
			ui.Text(c, "结果（编的）："+step["result"]).FontSize(13).TextColor(tk.TextSecondary)
		}
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
			ui.Scroll(c).Grow(1).Basis(0).TrackScroll(followEnd("new-product")).Children(func() {
				chatColumn(c, func() {
					ui.Column(c).Grow(1).Basis(0).MaxWidth(chatWidth).Padding(24, 0).Gap(14).Children(func() {
						for i, m := range messages {
							if m.Role == "user" {
								userMessage(c, "new-product", i, m.Text, !streaming, func(text string) { go app.chatIntakeEditResend(i, text) })
							} else {
								markdown.Markdown(c, breaks(m.Text), markdown.MarkdownProps{Variant: markdown.Chat})
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
			OnStop: func() { go app.stopIntake() },
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
