package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

// 和网页版共用的小部件：提示条、状态标签、任务进度、表格行、时间与数字格式。

// tone 状态标签的色调，和网页版 Chip 的 tone 对应。
type tone int

const (
	toneMuted tone = iota
	toneOk
	toneWarn
	toneAsk
	toneBad
)

func chip(c *ui.Context, text string, t tone) {
	color := display.TagColor("")
	switch t {
	case toneOk:
		color = display.TagSuccess
	case toneWarn:
		color = display.TagWarning
	case toneAsk:
		color = display.TagProcessing
	case toneBad:
		color = display.TagError
	}
	display.Tag(c, text, display.TagProps{Color: color, Size: theme.Small})
}

// decisionTone 组件选型的四种结论。
func decisionTone(d string) tone {
	switch d {
	case "必选":
		return toneOk
	case "二期":
		return toneWarn
	case "待确认":
		return toneAsk
	}
	return toneMuted
}

var decisionOrder = []string{"必选", "二期", "待确认", "放弃"}

var decisionHint = map[string]string{
	"必选":  "第一版就要做",
	"二期":  "以后再做",
	"待确认": "取决于你对待确认问题的回答",
	"放弃":  "这个产品用不上",
}

// callout 一条提示：标题、说明，可带一个按钮。kind 是 feedback.Alert 的类型。
func callout(c *ui.Context, kind feedback.AlertType, title, desc string, act func()) {
	ui.Column(c).Gap(8).Children(func() {
		feedback.Alert(c, title, feedback.AlertProps{Type: kind, Description: desc})
		if act != nil {
			ui.Row(c).Gap(8).Children(act)
		}
	})
}

// modeCallouts 离线演示 / 还没配模型的提示，每页都放在最上面（和网页版 ModeCallouts 一样）。
func modeCallouts(c *ui.Context) {
	app.mu.Lock()
	loaded, mock, configured := app.cfgLoaded, app.cfgMock, app.cfgConfigured
	app.mu.Unlock()
	switch {
	case !loaded:
	case mock:
		callout(c, feedback.Warning, "离线演示模式",
			"不调用模型，回复和方案都是按关键词拼出来的占位内容，用来看看流程长什么样，不能用于真实决策。", nil)
	case !configured:
		callout(c, feedback.Warning, "第一步：配置模型",
			"引擎要靠大模型来对话和推导方案。推荐 DeepSeek，便宜、国内可直连，充值几块钱就能用很久。", func() {
				action.Button(c, "去配置", action.ButtonProps{Variant: action.Primary, Size: theme.Small}).OnClick(gotoSettings)
			})
	}
}

// configured 模型是否可用（离线演示也算可用）。
func configured() bool {
	app.mu.Lock()
	defer app.mu.Unlock()
	return app.cfgConfigured
}

// section 一节：标题 + 右侧小字，下面是内容。和网页版的 Section 对应。
func section(c *ui.Context, title func(), aside func(), body func()) {
	ui.Column(c).Gap(12).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Grow(1).MinWidth(0).Children(title)
			if aside != nil {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Shrink(0).Children(aside)
			}
		})
		body()
	})
}

func sectionTitle(c *ui.Context, text string) {
	ui.Text(c, text).FontSize(16).FontWeight(600)
}

func muted(c *ui.Context, text string, size float32) ui.Element {
	tk := theme.TokensOf(c)
	return ui.Text(c, text).FontSize(size).TextColor(tk.TextTertiary)
}

func mono(c *ui.Context, text string, size float32) ui.Element {
	tk := theme.TokensOf(c)
	return ui.Text(c, text).Font("monospace").FontSize(size).TextColor(tk.TextTertiary)
}

// divided 一组行，行与行之间一条细线（表格、问题列表用）。
func divided(c *ui.Context, n int, row func(i int)) {
	tk := theme.TokensOf(c)
	ui.Column(c).Children(func() {
		for i := 0; i < n; i++ {
			r := ui.Column(c).Padding(10, 0)
			if i > 0 {
				r.BorderWidth(1, 0, 0, 0).BorderColor(tk.BorderSecondary)
			}
			r.Children(func() { row(i) })
		}
	})
}

// card 细边框圆角卡片。
func card(c *ui.Context, fn func()) {
	tk := theme.TokensOf(c)
	ui.Column(c).Padding(16, 18).Gap(10).Radius(12).Border(1, tk.BorderSecondary).Background(tk.BgContainer).Children(fn)
}

// fmtNum 数字加千位分隔（zh-CN 的写法）。
func fmtNum(n int) string {
	s := strconv.Itoa(n)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var b strings.Builder
	for i, ch := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(ch)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// timeAgo 和网页版一样：刚刚 / N 分钟前 / N 小时前 / M 月 D 日。
func timeAgo(epoch float64) string {
	if epoch <= 0 {
		return ""
	}
	t := time.Unix(int64(epoch), 0)
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "刚刚"
	case d < time.Hour:
		return fmt.Sprintf("%d 分钟前", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d 小时前", int(d.Hours()))
	}
	return fmt.Sprintf("%d 月 %d 日", int(t.Month()), t.Day())
}

func pad2(i int) string { return fmt.Sprintf("%02d", i) }

// -- 任务进度 ------------------------------------------------------------------

type stage struct{ mark, label, desc string }

var planStages = []stage{
	{"①", "需求拆解", "把想法拆成目标、用户、核心闭环等硬事实"},
	{"②", "领域建模", "对应到工具、知识、观察、行动、权限五要素"},
	{"③", "组件选型", "按固定规则决定 17 个组件选哪些，不调模型"},
	{"④", "架构设计", "工具清单、系统提示词、七层架构"},
	{"⑤", "独立审稿", "换个视角挑方案里的矛盾，不通过就打回重写一次"},
	{"⑥", "整理待确认问题", "改成大白话、合并重复、把最关键的排前面"},
	{"⑦", "整理需求文档", "把你说过的、答过的、新加的，写进需求文档各节"},
}

var auditStages = []stage{
	{"①", "盘点项目", "列出文件、依赖和入口，只在本机做，不调模型"},
	{"②", "读代码", "agent 读代码，反推这个项目在做什么，逐个核实 17 个组件"},
	{"③", "对照打分", "按固定规则算出它应该有什么，和现在的代码对照"},
	{"④", "独立复核", "换个视角，挑出报告里站不住的判断"},
}

type wording struct{ running, failed, done, hint, resume string }

var planWording = wording{"正在推导方案", "推导中断", "推导完成",
	"大约需要 2–5 分钟。可以先去做别的，切到别的页面也不影响。", "继续推导"}

var auditWording = wording{"正在检查", "检查中断", "检查完成",
	"大约需要 3–8 分钟，项目越大越久。可以先去做别的，切到别的页面也不影响。", "继续检查"}

type stageState struct {
	stage
	state    string // todo active done failed
	note     string
	activity string
}

// stageStates 从任务日志推出每一步的状态，规则和网页版 job-progress.tsx 相同。
func stageStates(job *JobInfo, stages []stage) []stageState {
	current, rewrite, activity := -1, false, ""
	for _, line := range job.Logs {
		idx := -1
		for i, s := range stages {
			if strings.Contains(line, s.mark) {
				idx = i
				break
			}
		}
		if idx >= 0 && (strings.Contains(line, "▶") || strings.Contains(line, "↻")) {
			if idx > current {
				activity = ""
			}
			if idx > current {
				current = idx
			}
		}
		if strings.Contains(line, "按审稿意见重写") {
			rewrite = true
		}
		if t := strings.TrimSpace(line); strings.HasPrefix(t, "· ") {
			activity = strings.TrimPrefix(t, "· ")
		}
	}
	out := make([]stageState, len(stages))
	for i, s := range stages {
		st := "todo"
		if job.Status == "done" || i < current {
			st = "done"
		} else if i == current {
			st = "active"
			if job.Status == "failed" {
				st = "failed"
			}
		}
		o := stageState{stage: s, state: st}
		if rewrite && i >= 3 {
			o.note = "第 2 版"
		}
		if st == "active" {
			o.activity = activity
		}
		out[i] = o
	}
	return out
}

func duration(sec float64) string {
	s := int(sec + 0.5)
	if s < 0 {
		s = 0
	}
	if s < 60 {
		return fmt.Sprintf("%d 秒", s)
	}
	return fmt.Sprintf("%d 分 %d 秒", s/60, s%60)
}

// jobProgress 任务进度卡：标题、用时、每一步的状态，失败时给「继续」和「检查模型设置」。
func jobProgress(c *ui.Context, job *JobInfo, stages []stage, w wording, compact bool, onResume func()) {
	tk := theme.TokensOf(c)
	running := job.Status == "running"
	end := job.Finished
	if end == 0 {
		end = float64(time.Now().UnixMilli()) / 1000
	}
	title := w.done
	switch {
	case running:
		title = w.running + "…"
	case job.Status == "failed":
		title = w.failed
	}
	box := ui.Column(c).Gap(6)
	if !compact {
		box.Padding(16, 18).Radius(12).Border(1, tk.BorderSecondary).Background(tk.BgContainer)
	}
	box.Children(func() {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			size := float32(15)
			if compact {
				size = 14
			}
			ui.Text(c, title).FontSize(size).FontWeight(600).Grow(1)
			mono(c, duration(end-job.Started), 12)
		})
		if running && !compact {
			muted(c, w.hint, 13)
		}
		states := stageStates(job, stages)
		ui.Column(c).Padding(4, 0, 0, 0).Children(func() {
			for i, s := range states {
				row := ui.Row(c).Gap(10).AlignItems(ui.Center).Padding(7, 0)
				if i > 0 {
					row.BorderWidth(1, 0, 0, 0).BorderColor(tk.BorderSecondary)
				}
				row.Children(func() {
					dot := ui.Column(c).Width(20).Height(20).Radius(10).Shrink(0).AlignItems(ui.Center).Justify(ui.Center)
					switch s.state {
					case "done":
						dot.Background(tk.Success).Children(func() {
							ui.Icon(c, lucide.Check()).Width(12).Height(12).TextColor(tk.BgContainer)
						})
					case "failed":
						dot.Background(tk.Error).Children(func() {
							ui.Icon(c, lucide.X()).Width(12).Height(12).TextColor(tk.BgContainer)
						})
					case "active":
						dot.Border(1, tk.Text).Children(func() {
							ui.Icon(c, lucide.LoaderCircle()).Width(12).Height(12).TextColor(tk.Text)
						})
					default:
						dot.Border(1, tk.Border)
					}
					mono(c, pad2(i+1), 12).Width(22).Shrink(0)
					ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(2).Children(func() {
						label := ui.Text(c, s.label).FontSize(14)
						switch s.state {
						case "todo":
							label.TextColor(tk.TextTertiary)
						case "active":
							label.FontWeight(600)
						}
						if !compact {
							if s.activity != "" {
								mono(c, s.activity, 12).MaxLines(1)
							} else {
								muted(c, s.desc, 12)
							}
						}
					})
					if s.note != "" {
						mono(c, s.note, 12)
					}
				})
			}
		})
		if job.Status == "failed" {
			ui.Column(c).Padding(10, 12).Gap(8).Radius(8).Background(tk.ErrorBg).Children(func() {
				ui.Text(c, job.Error).FontSize(13).TextColor(tk.ErrorText)
				if onResume != nil {
					ui.Row(c).Gap(8).Children(func() {
						action.Button(c, w.resume, action.ButtonProps{Variant: action.Primary, Size: theme.Small}).OnClick(onResume)
						action.Button(c, "检查模型设置", action.ButtonProps{Size: theme.Small}).OnClick(gotoSettings)
					})
				}
			})
		}
		if !compact {
			logs := strings.Join(job.Logs, "\n")
			if logs == "" {
				logs = "（还没有输出）"
			}
			display.Accordion(c, nil, display.AccordionProps{Items: []display.AccordionItem{{Key: "logs", Title: "详细日志"}}},
				func(string) {
					ui.Column(c).Padding(10, 12).Radius(8).Background(tk.FillQuaternary).Children(func() {
						ui.Text(c, logs).Font("monospace").FontSize(12).TextColor(tk.TextSecondary)
					})
				})
		}
	})
}
