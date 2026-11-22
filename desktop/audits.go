package main

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/markdown"
	"github.com/lobehub/lobe-mygo/pkg/overlay"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

// 项目检查页：左边检查过的项目，右边一份报告。功能和网页版 audits.tsx 一致。

var reVerdict = regexp.MustCompile(`## 一句话判定\s+\*\*(.+?)。?\*\*`)

// auditVerdict 报告「一句话判定」里加粗的那句。
func auditVerdict(report string) string {
	if m := reVerdict.FindStringSubmatch(report); m != nil {
		return m[1]
	}
	return ""
}

// cleanReport 和网页版一样整理报告：去掉一级标题和「一句话判定」（摘要卡里已经有了），
// 表头含「去哪看」的表格去掉最后一列（那一列是给终端用户看的文件路径提示）。
func cleanReport(report string) string {
	lines := strings.Split(report, "\n")
	out := make([]string, 0, len(lines))
	titleDone, skipping, dropLast := false, false, false
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if !titleDone && strings.HasPrefix(t, "# ") {
			titleDone = true
			continue
		}
		if strings.HasPrefix(t, "## ") {
			skipping = strings.HasPrefix(t, "## 一句话判定")
		}
		if skipping {
			continue
		}
		if strings.HasPrefix(t, "|") {
			if strings.Contains(t, "| 去哪看 |") {
				dropLast = true
			}
			if dropLast {
				cells := strings.Split(strings.Trim(t, "|"), "|")
				if len(cells) > 1 {
					l = "|" + strings.Join(cells[:len(cells)-1], "|") + "|"
				}
			}
		} else {
			dropLast = false
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

func completionTone(a AuditSummary) tone {
	switch {
	case a.Completion >= 0.8:
		return toneOk
	case a.Completion >= 0.3:
		return toneWarn
	}
	return toneBad
}

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
	tab := app.auditTab
	app.mu.Unlock()
	empty := len(audits) == 0 && auditsErr == ""
	running := job != nil && job.Status == "running"
	openNew := func() {
		app.mu.Lock()
		app.newAuditOpen, app.auditStatus = true, ""
		app.mu.Unlock()
		app.invalidate()
	}

	pageShell(c, func() {
		ui.Row(c).Grow(1).MinWidth(0).Gap(8).AlignItems(ui.Center).Children(func() {
			if detail != nil {
				ui.Text(c, detail.Name).FontSize(15).Bold().MaxLines(1).Shrink(0)
				mono(c, detail.Root, 12).MaxLines(1)
			} else {
				pageTitle(c, "检查过的项目", "")
				if len(audits) > 0 {
					mono(c, fmt.Sprint(len(audits)), 12)
				}
			}
		})
		if detail != nil {
			open := action.Button(c, "打开文件夹", action.ButtonProps{Size: theme.Small})
			open.OnClick(func() {
				go postJSON(app.baseURL(), "/api/audits/"+url.PathEscape(selected)+"/reveal", "{}")
			})
			overlay.Tooltip(c, open, detail.Path, overlay.TooltipProps{})
			if detail.Finished {
				can := !running && !busy && configured()
				re := action.Button(c, "重新判断", action.ButtonProps{Size: theme.Small})
				re.Disabled(!can)
				if can {
					re.OnClick(func() {
						app.mu.Lock()
						app.rejudgeOpen = true
						app.mu.Unlock()
						app.invalidate()
					})
				}
				again := action.Button(c, "重新检查", action.ButtonProps{Size: theme.Small})
				again.Disabled(!can)
				if can {
					again.OnClick(func() { go app.runAudit(selected, "recheck") })
				}
				overlay.Tooltip(c, again, "代码改过之后再查一次", overlay.TooltipProps{})
			}
		}
		if !empty {
			action.Button(c, "检查一个项目", action.ButtonProps{Variant: action.Primary, Size: theme.Small}).OnClick(openNew)
		}
	}, func() {
		if empty {
			productScroll(c, false, func() {
				modeCallouts(c)
				ui.Column(c).Gap(10).Padding(24, 0, 0, 0).Children(func() {
					ui.Text(c, "看看一个已有的 Agent 项目还差什么").FontSize(26).Bold()
					ui.Text(c, "选一个项目文件夹，引擎让 agent 读代码，告诉你它离一个靠谱的 Agent 还差哪些机制，并反推一份它本该有的需求文档。").
						FontSize(15).TextColor(tk.TextSecondary)
				})
				ui.Row(c).Children(func() {
					action.Button(c, "检查一个项目 →", action.ButtonProps{Variant: action.Primary}).OnClick(openNew)
				})
			})
			newAuditDialog(c)
			return
		}
		ui.Row(c).Grow(1).Basis(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			// 左：检查过的项目
			ui.Column(c).Width(260).Shrink(0).Padding(12, 8).Gap(4).
				BorderWidth(0, 1, 0, 0).BorderColor(tk.BorderSecondary).Children(func() {
				if auditsErr != "" {
					errorAlert(c, auditsErr)
				}
				items := make([]display.ListItem, 0, len(audits))
				for _, a := range audits {
					desc := a.Verdict
					switch {
					case !a.Finished:
						desc = "没跑完"
					case desc == "":
						desc = "已检查"
					}
					items = append(items, display.ListItem{
						Key: a.Slug, Label: a.Name, Description: desc, Icon: lucide.FolderSearch(),
						Extra: fmt.Sprintf("%.0f%%", a.Completion*100),
					})
				}
				if clicked := display.List(c, display.ListProps{Items: items, Active: selected}); clicked != "" {
					go app.selectAudit(clicked)
				}
				ui.Column(c).Grow(1)
				muted(c, "检查结果在引擎目录的 out/audit-项目名/ 下；在终端里用 blueprint audit 检查过的项目也会列在这里。", 11).Padding(8)
			})
			// 右：报告
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
				if selected == "" {
					centerEmpty(c, display.EmptyProps{Icon: lucide.FileText(), Title: "选一个检查结果",
						Description: "左边选一个项目，这里显示它的差距报告。"})
					return
				}
				productScroll(c, false, func() {
					modeCallouts(c)
					if detailErr != "" {
						errorAlert(c, detailErr)
						return
					}
					if job != nil && job.Status != "done" {
						jobProgress(c, job, auditStages, auditWording, false, func() { go app.runAudit(selected, "resume") })
					}
					if detail == nil {
						mutedText(c, "读取中…")
						return
					}
					if !detail.Finished {
						if job == nil {
							callout(c, feedback.Info, "上次检查没跑完", "接着上次中断的地方跑，已完成的步骤不会重复调用模型。", func() {
								b := action.Button(c, "继续检查", action.ButtonProps{Variant: action.Primary, Size: theme.Small})
								b.Disabled(busy || !configured())
								if !busy && configured() {
									b.OnClick(func() { go app.runAudit(selected, "resume") })
								}
							})
						}
						return
					}
					if running {
						callout(c, feedback.Info, "下面还是上一次的报告", "这次检查完成后会自动刷新。", nil)
					}
					auditSummary(c, detail)
					t := tab
					action.Segmented(c, &t, []string{"差距报告", "本该有的需求文档"}, action.SegmentedProps{Size: theme.Small})
					if t != tab {
						app.mu.Lock()
						app.auditTab = t
						app.mu.Unlock()
					}
					if t == 0 {
						markdown.Markdown(c, breaks(cleanReport(detail.Report)), markdown.MarkdownProps{})
					} else {
						muted(c, "按代码反推出来的需求文档：这个项目如果从头写需求，大概应该写成这样。缺的机制按差距报告补进去了。", 13)
						markdown.Markdown(c, breaks(detail.SuggestedPRD), markdown.MarkdownProps{})
					}
				})
			})
		})
		rejudgeDialog(c, selected)
		newAuditDialog(c)
	})
}

// auditSummary 报告顶上的摘要卡：一句话判定、覆盖率、缺几项弱几项、怎么检查的。
func auditSummary(c *ui.Context, d *AuditDetail) {
	tk := theme.TokensOf(c)
	card(c, func() {
		verdict := auditVerdict(d.Report)
		if verdict == "" {
			verdict = "已检查"
		}
		ui.Row(c).Gap(10).AlignItems(ui.Center).Wrap().Children(func() {
			ui.Text(c, verdict).FontSize(20).Bold()
			muted(c, fmt.Sprintf("必选组件信号覆盖率 %.0f%%", d.Completion*100), 13)
		})
		if d.Title != "" && d.Title != d.Name {
			ui.Text(c, d.Title).FontSize(14).TextColor(tk.TextSecondary)
		}
		ui.Row(c).Gap(6).Children(func() {
			if d.Missing > 0 {
				chip(c, fmt.Sprintf("缺 %d 项", d.Missing), toneBad)
			}
			if d.Weak > 0 {
				chip(c, fmt.Sprintf("弱 %d 项", d.Weak), toneWarn)
			}
			if d.Missing == 0 && d.Weak == 0 {
				chip(c, "该有的都有", toneOk)
			}
		})
		if d.Mode != "agent" && d.ModeNote != "" {
			ui.Text(c, "没用上 agent："+d.ModeNote).FontSize(13).TextColor(tk.WarningText)
		}
		facts := []string{"快速扫描（按关键词判断，可能误判）"}
		if d.Mode == "agent" {
			facts[0] = fmt.Sprintf("agent 读了 %d 个文件", d.FilesSeen)
		}
		if d.FeaturesFrom != "" {
			facts = append(facts, fmt.Sprintf("「该有什么」沿用 %s 那次的判断", d.FeaturesFrom))
		}
		if d.Calls > 0 {
			facts = append(facts, fmt.Sprintf("调用模型 %d 次", d.Calls))
		}
		if ago := timeAgo(d.Updated); ago != "" {
			facts = append(facts, ago)
		}
		muted(c, strings.Join(facts, " · "), 12)
	})
}

// rejudgeDialog 「重新判断」的确认框。
func rejudgeDialog(c *ui.Context, slug string) {
	app.mu.Lock()
	open := app.rejudgeOpen
	app.mu.Unlock()
	before := open
	ok := feedback.Modal(c, &open, feedback.ModalProps{Title: "重新判断这个项目该有什么？", OkText: "重新判断", CancelText: "取消"}, func() {
		ui.Text(c, "「重新检查」会沿用上次对「这个项目该有哪些机制」的判断，结果只随代码变化。重新判断会让模型从头再判断一次，结果可能和上次有一两处不同。项目本身的定位变了（比如加了新功能）时再用它。").
			FontSize(14).LineHeight(1.7)
	})
	if ok {
		open = false
		go app.runAudit(slug, "rejudge")
	}
	if open != before {
		app.mu.Lock()
		app.rejudgeOpen = open
		app.mu.Unlock()
	}
}

// newAuditDialog 「检查一个项目」：选文件夹或粘贴路径，再开始检查。
func newAuditDialog(c *ui.Context) {
	tk := theme.TokensOf(c)
	app.mu.Lock()
	open, path, busy, status := app.newAuditOpen, app.newAuditPath, app.auditBusy, app.auditStatus
	app.mu.Unlock()
	before, pathBefore := open, path
	feedback.Modal(c, &open, feedback.ModalProps{Title: "检查一个已有的 Agent 项目", Width: 560, NoFooter: true}, func() {
		ui.Column(c).Gap(14).Children(func() {
			ui.Text(c, "项目文件夹").FontSize(13).FontWeight(500)
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				label := "选择文件夹…"
				if busy && status == "在弹出的窗口里选…" {
					label = status
				}
				b := action.Button(c, label, action.ButtonProps{})
				b.Disabled(busy)
				if !busy {
					b.OnClick(func() { go app.pickAuditFolder() })
				}
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
					field.Input(c, &path, field.InputProps{Placeholder: "或者把文件夹路径粘贴到这里"})
				})
			})
			muted(c, "粘贴路径：在 Finder 里按住 Option 键右键文件夹，选「将…拷贝为路径名称」。", 12)
			for _, n := range [][2]string{
				{"只读，不改你的代码", "也不会运行它。agent 只能读这个文件夹里的文件。"},
				{"代码会发给你配置的模型服务商", "agent 读到的代码会随请求发出去；.env 和私钥文件不读。"},
				{"大约 3–8 分钟", "最多调用模型 40 次。中途断了可以接着跑，已完成的步骤不重复花钱。"},
			} {
				ui.Column(c).Gap(2).Children(func() {
					ui.Text(c, n[0]).FontSize(13).FontWeight(500)
					muted(c, n[1], 12)
				})
			}
			if status != "" && status != "在弹出的窗口里选…" && status != "正在开始…" {
				ui.Text(c, status).FontSize(13).TextColor(tk.ErrorText)
			}
			ui.Row(c).Justify(ui.End).Children(func() {
				label := "开始检查"
				if busy && status == "正在开始…" {
					label = status
				}
				can := strings.TrimSpace(path) != "" && !busy && configured()
				b := action.Button(c, label, action.ButtonProps{Variant: action.Primary})
				b.Disabled(!can)
				if can {
					b.OnClick(func() { go app.startAudit() })
				}
			})
		})
	})
	if open != before || path != pathBefore {
		app.mu.Lock()
		app.newAuditOpen, app.newAuditPath = open, path
		app.mu.Unlock()
	}
}
