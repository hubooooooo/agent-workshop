package main

import (
	"fmt"
	"math"
	"net/url"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/markdown"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

// 产品页的三段对话：回答问题、加新需求、试玩。布局和网页版 ChatPane 一样：
// 顶部一条说明 +「重新开始」，可选的提示条，下面是对话和输入框。

// chatStrip 对话页顶部的一条：左边说明，右边「重新开始」。
func chatStrip(c *ui.Context, hint string, canRestart bool, onRestart func()) {
	tk := theme.TokensOf(c)
	ui.Row(c).Shrink(0).Height(36).Padding(0, 20).Gap(12).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(tk.BorderSecondary).Children(func() {
		muted(c, hint, 12).Grow(1).Basis(0).MinWidth(0).MaxLines(1)
		if canRestart {
			action.Button(c, "重新开始", action.ButtonProps{Variant: action.Text, Size: theme.Small}).OnClick(onRestart)
		}
	})
}

// chatScrolls 每段对话的滚动位置。只在界面线程里读写。
var chatScrolls = map[string]*ui.ScrollState{}

// followEnd 对话区跟着最新的消息走：停在底部时，新内容进来就继续贴底；用户往上翻了就不打扰。
func followEnd(key string) *ui.ScrollState {
	st := chatScrolls[key]
	if st == nil {
		st = &ui.ScrollState{Y: math.MaxFloat32}
		chatScrolls[key] = st
	}
	if st.Y >= st.MaxY-8 {
		st.Y = math.MaxFloat32
	}
	return st
}

// chatColumnScroll 对话区：可滚动、居中一列，跟着最新消息滚到底。
func chatColumnScroll(c *ui.Context, key string, fn func()) {
	ui.Scroll(c).Grow(1).Basis(0).TrackScroll(followEnd(key)).Children(func() {
		chatColumn(c, func() {
			ui.Column(c).Grow(1).Basis(0).MaxWidth(chatWidth).Padding(24, 0).Gap(14).Children(fn)
		})
	})
}

// chipRow 一排快捷回复，点了就把这句原样发出去。
func chipRow(c *ui.Context, chips []string, send func(string)) {
	if len(chips) == 0 {
		return
	}
	ui.Row(c).Gap(8).Wrap().Children(func() {
		for _, ch := range chips {
			action.Button(c, ch, action.ButtonProps{Size: theme.Small}).OnClick(func() { send(ch) })
		}
	})
}

// sessionView 一段 chatSession 对话：消息、卡片、快捷回复、错误、输入框（或「这段对话已经完成。」）。
func sessionView(c *ui.Context, key, endpoint, hint, placeholder string, cs chatSession, top func(), cardView func()) {
	tk := theme.TokensOf(c)
	send := func(text string) { go app.chatSend(key, endpoint, text) }
	chatStrip(c, hint, !cs.Streaming && cs.hasUser() && !cs.Finished, func() { go app.chatRestart(key) })
	if top != nil {
		top()
	}
	chatColumnScroll(c, key, func() {
		markdown.Markdown(c, breaks(cs.Intro), markdown.MarkdownProps{Variant: markdown.Chat})
		for i, m := range cs.Msgs {
			if m.Role == "user" {
				userBubble(c, m.Text)
			} else if m.Text != "" {
				markdown.Markdown(c, breaks(m.Text), markdown.MarkdownProps{Variant: markdown.Chat})
				last := i == len(cs.Msgs)-1 && !cs.Streaming && !cs.Finished
				replyActions(c, m.Text, last, func() { go app.chatRegenerate(key, endpoint) })
			}
		}
		if cs.Streaming && (len(cs.Msgs) == 0 || cs.Msgs[len(cs.Msgs)-1].Role == "user") {
			muted(c, "正在想…", 13)
		}
		if cs.Card != nil && cardView != nil {
			cardView()
		}
		if cs.Err != "" {
			errorAlert(c, cs.Err)
		}
		if !cs.Streaming && !cs.Finished && cs.Card == nil {
			chipRow(c, cs.chips(), send)
		}
	})
	if cs.Finished {
		ui.Row(c).Shrink(0).Justify(ui.Center).Padding(14, 0, 20, 0).Children(func() {
			ui.Text(c, "这段对话已经完成。").FontSize(13).TextColor(tk.TextTertiary)
		})
		return
	}
	draft := cs.Draft
	if text := chatComposer(c, cs.InputGen, &draft, composerProps{
		Placeholder: placeholder, Busy: cs.Streaming, BusyText: "正在回复…",
	}); text != "" {
		send(text)
		return
	}
	if draft != cs.Draft {
		app.mu.Lock()
		if live := app.chats[key]; live != nil {
			live.Draft = draft
		}
		app.mu.Unlock()
	}
}

// startedCard 卡片确认后：任务的简要进度。
func startedCard(c *ui.Context, slug string, cs chatSession) {
	app.mu.Lock()
	job := cs.Started
	if app.job != nil && job != nil && app.job.ID == job.ID {
		job = app.job // 用产品页轮询到的最新进度
	}
	app.mu.Unlock()
	card(c, func() {
		ui.Text(c, cs.StartedAs).FontSize(14).FontWeight(600)
		if job != nil {
			jobProgress(c, job, planStages, planWording, true, nil)
			if job.Status == "failed" {
				muted(c, "到概览里点「继续推导」可以从断点接着跑。", 12)
			}
		}
		label := "打开概览"
		if job != nil && job.Status == "done" {
			label = "查看方案"
		}
		ui.Row(c).Children(func() {
			action.Button(c, label, action.ButtonProps{Size: theme.Small}).OnClick(func() { setTab(slug, "overview") })
		})
	})
}

// -- 回答问题 ------------------------------------------------------------------

func answersView(c *ui.Context, slug string, plan *PlanView, job *JobInfo) {
	app.mu.Lock()
	intro := app.answerIntro
	key := "answers:" + slug + ":" + fingerprint(plan.Unknowns)
	cs := app.chatFor(key, "answers:"+slug+":", intro, []string{"帮我想想", "跳过这题"})
	snap := *cs
	app.mu.Unlock()

	running := job != nil && job.Status == "running"
	switch {
	case len(plan.Unknowns) == 0:
		productScroll(c, false, func() {
			callout(c, feedback.Success, "没有待确认的问题", "方案里的信息很完整，不需要再补充。", nil)
		})
		return
	case !configured():
		productScroll(c, false, func() { modeCallouts(c) })
		return
	case running && !snap.Finished:
		productScroll(c, false, func() {
			callout(c, feedback.Info, "正在按你的回答重新推导", "推导完成后，这里会换成新方案里的问题。", nil)
		})
		return
	}
	endpoint := "/api/products/" + url.PathEscape(slug) + "/chat"
	sessionView(c, key, endpoint, "回答只保存在这次打开的桌面版里，关掉应用前记得点「先保存已回答的」。", "输入你的回答…",
		snap, nil, func() {
			if snap.Finished {
				startedCard(c, slug, snap)
				return
			}
			answersCard(c, key, slug, snap)
		})
}

// answersCard 回答汇总卡：每条回答都还能改，确认后写进需求文档并重新推导。
func answersCard(c *ui.Context, key, slug string, cs chatSession) {
	tk := theme.TokensOf(c)
	card(c, func() {
		ui.Text(c, fmt.Sprintf("回答汇总 · %d 条", len(cs.Answers))).FontSize(14).FontWeight(600)
		for i, qa := range cs.Answers {
			ui.Column(c).Gap(6).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Start).Children(func() {
					mono(c, pad2(i+1), 12).Shrink(0)
					ui.Text(c, qa.Question).FontSize(13).TextColor(tk.TextSecondary).Grow(1).Basis(0).MinWidth(0)
				})
				ans := qa.Answer
				ui.Column(c).Children(func() {
					c.Key(fmt.Sprintf("a-%d-%d", cs.CardGen, i))
					field.TextArea(c, &ans, field.TextAreaProps{MinRows: 1, MaxRows: 6})
				})
				if ans != qa.Answer {
					app.mu.Lock()
					if live := app.chats[key]; live != nil && i < len(live.Answers) {
						live.Answers[i].Answer = ans
					}
					app.mu.Unlock()
				}
			})
		}
		if cs.CardErr != "" {
			ui.Text(c, cs.CardErr).FontSize(13).TextColor(tk.ErrorText)
		}
		ui.Row(c).Children(func() {
			label := "保存并重新推导方案"
			if cs.CardBusy {
				label = "正在保存…"
			}
			b := action.Button(c, label, action.ButtonProps{Variant: action.Primary})
			b.Disabled(cs.CardBusy)
			if !cs.CardBusy {
				b.OnClick(func() { go app.confirmAnswers(key, slug) })
			}
		})
	})
}

// -- 加新需求 ------------------------------------------------------------------

func reviseView(c *ui.Context, slug string, version int, job *JobInfo) {
	next := version + 1
	intro := fmt.Sprintf("想加什么新功能、改什么规则？直接说就行，比如「每周日晚上给家长发一份本周小结」。\n\n"+
		"我最多再问你一个问题，然后整理成一份草稿给你确认。确认后需求文档升到 V%d，方案按新需求重新推导，没变的部分不重复花钱。", next)
	app.mu.Lock()
	key := fmt.Sprintf("revise:%s:v%d", slug, version)
	cs := app.chatFor(key, "revise:"+slug+":", intro, nil)
	if seed := app.reviseSeed[slug]; seed != "" && !cs.Streaming && !cs.Finished {
		cs.Draft = seed // 试玩里记下的「这里不对」：放进输入框，用户补充后自己发
		cs.InputGen++
		delete(app.reviseSeed, slug)
	}
	snap := *cs
	app.mu.Unlock()

	running := job != nil && job.Status == "running"
	switch {
	case !configured():
		productScroll(c, false, func() { modeCallouts(c) })
		return
	case running && !snap.Finished:
		productScroll(c, false, func() {
			callout(c, feedback.Info, "正在推导", "等这一轮推导完成，再加新需求。", nil)
		})
		return
	}
	endpoint := "/api/products/" + url.PathEscape(slug) + "/revise-chat"
	sessionView(c, key, endpoint, fmt.Sprintf("当前是 V%d。加的新需求会记进版本历史，随时能回看。", version), "说说新需求…",
		snap, nil, func() {
			if snap.Finished {
				startedCard(c, slug, snap)
				return
			}
			revisionCard(c, key, slug, next, snap)
		})
}

// revisionCard 需求变更草稿：标题和内容都能改，确认后升一版。
func revisionCard(c *ui.Context, key, slug string, next int, cs chatSession) {
	tk := theme.TokensOf(c)
	card(c, func() {
		ui.Text(c, fmt.Sprintf("需求变更草稿 · V%d", next)).FontSize(14).FontWeight(600)
		title, req := cs.Title, cs.Requirement
		formField(c, "一句话标题", "", func() {
			ui.Column(c).Children(func() {
				c.Key(fmt.Sprintf("t-%d", cs.CardGen))
				field.Input(c, &title, field.InputProps{})
			})
		})
		formField(c, "新需求", "", func() {
			ui.Column(c).Children(func() {
				c.Key(fmt.Sprintf("r-%d", cs.CardGen))
				field.TextArea(c, &req, field.TextAreaProps{MinRows: 3, MaxRows: 10})
			})
		})
		if title != cs.Title || req != cs.Requirement {
			if r := []rune(title); len(r) > 40 {
				title = string(r[:40])
			}
			app.mu.Lock()
			if live := app.chats[key]; live != nil {
				live.Title, live.Requirement = title, req
			}
			app.mu.Unlock()
		}
		muted(c, fmt.Sprintf("确认后，需求文档升到 V%d（旧版本会留一份），方案按新需求重新推导；没变的部分沿用上次的结果，不重复花钱。", next), 12)
		if cs.CardErr != "" {
			ui.Text(c, cs.CardErr).FontSize(13).TextColor(tk.ErrorText)
		}
		ui.Row(c).Children(func() {
			label := fmt.Sprintf("升到 V%d 并重新推导", next)
			if cs.CardBusy {
				label = "正在升级…"
			}
			b := action.Button(c, label, action.ButtonProps{Variant: action.Primary})
			b.Disabled(cs.CardBusy)
			if !cs.CardBusy {
				b.OnClick(func() { go app.confirmRevision(key, slug, next) })
			}
		})
	})
}

// -- 试玩 ----------------------------------------------------------------------

func playPanel(c *ui.Context, slug string) {
	tk := theme.TokensOf(c)
	app.mu.Lock()
	intro := app.playIntro
	playErr := app.playErr
	messages := append([]PlayMsg(nil), app.playMessages...)
	streaming := app.playStreaming
	draft := app.playDraft
	draftBefore := draft
	inputGen := app.playInputGen
	chips := append([]string(nil), app.playChips...)
	role := app.playRole
	notes := append([]playNote(nil), app.notes[slug]...)
	noteOpen, noteDraft, noteGen := app.noteOpen, app.noteDraft, app.noteGen
	started := app.playStarted
	app.mu.Unlock()
	if !started && configured() {
		go app.startPlay(slug)
	}

	switch {
	case !configured():
		productScroll(c, false, func() { modeCallouts(c) })
		return
	case intro == "" && len(messages) == 0 && playErr != "":
		productScroll(c, false, func() {
			callout(c, feedback.Error, "试玩没能开始", playErr, func() {
				if strings.Contains(playErr, "模型设置") {
					action.Button(c, "去模型设置", action.ButtonProps{Variant: action.Primary, Size: theme.Small}).OnClick(gotoSettings)
				}
			})
		})
		return
	case intro == "":
		ui.Column(c).Grow(1).Basis(0).Children(func() {
			centerEmpty(c, display.EmptyProps{Icon: lucide.MessagesSquare(), Title: "正在准备试玩…",
				Description: "Agent 会先说明它扮演谁、能帮你做什么，然后你以使用者的身份跟它聊。"})
		})
		return
	}

	hasUser := false
	for _, m := range messages {
		if m.Role == "user" {
			hasUser = true
		}
	}
	chatStrip(c, "这是按方案模拟的，不是做出来的产品：工具结果都是编的，真正的产品要开发者按交接包来做。",
		!streaming && hasUser, func() { go app.restartPlay(slug) })
	if len(notes) > 0 {
		ui.Row(c).Shrink(0).Padding(8, 20).Gap(12).AlignItems(ui.Center).Background(tk.WarningBg).
			BorderWidth(0, 0, 1, 0).BorderColor(tk.BorderSecondary).Children(func() {
			ui.Text(c, fmt.Sprintf("试玩中记下了 %d 处不对", len(notes))).FontSize(13).TextColor(tk.WarningText).Grow(1)
			action.Button(c, "带去加新需求 →", action.ButtonProps{Size: theme.Small}).OnClick(func() { go app.carryNotes(slug) })
		})
	}
	chatColumnScroll(c, "play:"+slug, func() {
		markdown.Markdown(c, breaks(intro), markdown.MarkdownProps{Variant: markdown.Chat})
		for _, m := range messages {
			switch m.Role {
			case "user":
				userBubble(c, m.Text)
			case "step":
				stepCard(c, m.Step)
			default:
				markdown.Markdown(c, breaks(m.Text), markdown.MarkdownProps{Variant: markdown.Chat})
				if m.Feedback && !(streaming && m.ID == lastAssistantID(messages)) {
					noteControl(c, slug, m.ID, notes, noteOpen, noteDraft, noteGen)
				}
				replyActions(c, m.Text, !streaming && m.ID == lastAssistantID(messages), func() { go app.regeneratePlay() })
			}
		}
		if streaming && (len(messages) == 0 || messages[len(messages)-1].Role == "user") {
			muted(c, "正在按方案跑…", 13)
		}
		if playErr != "" {
			errorAlert(c, playErr)
		}
		if !streaming && !hasUser {
			chipRow(c, chips, func(text string) { go app.sendPlay(text) })
		}
	})
	if text := chatComposer(c, inputGen, &draft, composerProps{
		Placeholder: fmt.Sprintf("以「%s」的身份说点什么…", role), Busy: streaming, BusyText: "Agent 正在按方案跑工具…",
	}); text != "" {
		draft = ""
		app.mu.Lock()
		app.playInputGen++
		app.mu.Unlock()
		go app.sendPlay(text)
	}
	writeBack(&app.playDraft, draftBefore, draft)
}

// replyActions 回复下面的「复制」，最后一条还有「重新生成」，和网页版一样。
func replyActions(c *ui.Context, text string, last bool, regenerate func()) {
	ui.Row(c).Gap(2).Margin(-6, 0, 0, 0).Children(func() {
		action.CopyButton(c, text, action.CopyButtonProps{Size: theme.Small})
		if last {
			action.ActionIcon(c, lucide.RefreshCw(), action.ActionIconProps{Title: "重新生成", Size: theme.Small}).OnClick(regenerate)
		}
	})
}

func lastAssistantID(msgs []PlayMsg) int {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "assistant" {
			return msgs[i].ID
		}
	}
	return 0
}

// noteControl 回复下面的「这里不对」：没记过是一个按钮，展开是输入框，记过了显示记下的话。
func noteControl(c *ui.Context, slug string, id int, notes []playNote, open int, draft string, gen int) {
	tk := theme.TokensOf(c)
	for _, n := range notes {
		if n.ID == id {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, lucide.Check()).Width(14).Height(14).TextColor(tk.SuccessText)
				ui.Text(c, "已记下："+n.Comment).FontSize(13).TextColor(tk.TextSecondary).Grow(1).Basis(0).MinWidth(0)
				action.ActionIcon(c, lucide.X(), action.ActionIconProps{Title: "删掉这条"}).
					OnClick(func() { go app.removeNote(slug, id) })
			})
			return
		}
	}
	if open != id {
		ui.Row(c).Children(func() {
			action.Button(c, "这里不对", action.ButtonProps{Variant: action.Text, Size: theme.Small}).OnClick(func() {
				app.mu.Lock()
				app.noteOpen, app.noteDraft = id, ""
				app.noteGen++
				app.mu.Unlock()
				app.invalidate()
			})
		})
		return
	}
	text := draft
	ui.Column(c).Gap(8).Children(func() {
		ui.Column(c).Children(func() {
			c.Key(fmt.Sprintf("note-%d", gen))
			field.TextArea(c, &text, field.TextAreaProps{Placeholder: "你希望它怎么做？比如「打折的事要直接说做不了，不用转给我」", MinRows: 2})
		})
		ui.Row(c).Gap(8).Children(func() {
			b := action.Button(c, "记下", action.ButtonProps{Variant: action.Primary, Size: theme.Small})
			ok := strings.TrimSpace(text) != ""
			b.Disabled(!ok)
			if ok {
				b.OnClick(func() { go app.saveNote(slug, id, text) })
			}
			action.Button(c, "取消", action.ButtonProps{Size: theme.Small}).OnClick(func() {
				app.mu.Lock()
				app.noteOpen, app.noteDraft = 0, ""
				app.mu.Unlock()
				app.invalidate()
			})
		})
	})
	if text != draft {
		app.mu.Lock()
		app.noteDraft = text
		app.mu.Unlock()
	}
}
