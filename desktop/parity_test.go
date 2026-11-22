package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestBreaksKeepsSingleNewlines(t *testing.T) {
	src := "A. 第一种\nB. 第二种\n\n```\ncode\nmore\n```\n| a | b |\n| - | - |\nend"
	got := breaks(src)
	if !strings.Contains(got, "A. 第一种  \nB. 第二种") {
		t.Fatalf("single newline should become a hard break, got %q", got)
	}
	if !strings.Contains(got, "```\ncode\nmore\n```") {
		t.Fatalf("code blocks must stay as they are, got %q", got)
	}
	if !strings.Contains(got, "| a | b |\n| - | - |") {
		t.Fatalf("table rows must stay as they are, got %q", got)
	}
}

func TestCleanReportLikeTheWeb(t *testing.T) {
	report := "# Agent Harness 差距报告\n\n## 一句话判定\n\n**只有雏形。** 覆盖率 38%。\n\n## 它现在是什么\n\n正文\n\n" +
		"| 组件 | 现状 | 去哪看 |\n| --- | --- | --- |\n| s03 | 缺失 | src/bot.py |\n\n收尾"
	got := cleanReport(report)
	if strings.Contains(got, "# Agent Harness") || strings.Contains(got, "一句话判定") || strings.Contains(got, "只有雏形") {
		t.Fatalf("title and verdict section should be gone, got %q", got)
	}
	if strings.Contains(got, "去哪看") || strings.Contains(got, "src/bot.py") {
		t.Fatalf("the 去哪看 column should be dropped, got %q", got)
	}
	if !strings.Contains(got, "| s03 | 缺失 |") || !strings.Contains(got, "## 它现在是什么") || !strings.Contains(got, "收尾") {
		t.Fatalf("the rest of the report must stay, got %q", got)
	}
	if v := auditVerdict(report); v != "只有雏形" {
		t.Fatalf("verdict = %q", v)
	}
}

func TestStageStatesFromLogs(t *testing.T) {
	job := &JobInfo{Status: "running", Logs: []string{
		"  ▶ ① 需求拆解", "  ▶ ③ 组件选型（规则）", "  ▶ ④ 领域建模 + 架构设计（agent）", "    · 查组件说明 s03",
	}}
	st := stageStates(job, planStages)
	if st[0].state != "done" || st[2].state != "done" || st[3].state != "active" || st[4].state != "todo" {
		t.Fatalf("states = %+v", st)
	}
	if st[3].activity != "查组件说明 s03" {
		t.Fatalf("active stage should show the latest activity, got %q", st[3].activity)
	}
	job.Status = "failed"
	if st := stageStates(job, planStages); st[3].state != "failed" {
		t.Fatalf("the stage that was running should be failed, got %s", st[3].state)
	}
}

func TestCarryNotesSeedsRevise(t *testing.T) {
	s := newState()
	s.playMessages = []PlayMsg{
		{Role: "user", Text: "能打折吗"},
		{Role: "assistant", ID: 7, Text: "我去问问店主", Feedback: true},
	}
	s.saveNote("bakery", 7, "  打折的事直接说做不了  ")
	if n := s.notes["bakery"]; len(n) != 1 || n[0].Said != "能打折吗" || n[0].Reply != "我去问问店主" || n[0].Comment != "打折的事直接说做不了" {
		t.Fatalf("note = %+v", n)
	}
	s.carryNotes("bakery")
	want := "试玩时发现这些地方和我想的不一样：\n1. 我说「能打折吗」，它回「我去问问店主」。我希望：打折的事直接说做不了"
	if s.reviseSeed["bakery"] != want {
		t.Fatalf("seed = %q", s.reviseSeed["bakery"])
	}
	if len(s.notes["bakery"]) != 0 || s.productTab != "revise" {
		t.Fatalf("carrying should clear the notes and open 加新需求, tab=%s", s.productTab)
	}
}

func TestReviseJobFinishGoesToHistory(t *testing.T) {
	s := newState()
	s.onPlanJobFinished(&JobInfo{Kind: "revise", Status: "done", Result: map[string]any{
		"new_version": float64(2), "diff": []any{map[string]any{"id": "s09"}},
	}})
	if s.productTab != "history" || len(s.toasts) != 1 || s.toasts[0].text != "已升到 V2，组件选型变了 1 处" {
		t.Fatalf("tab=%s toasts=%+v", s.productTab, s.toasts)
	}
	s = newState()
	s.onPlanJobFinished(&JobInfo{Kind: "plan", Status: "done"})
	if s.productTab != "overview" || s.toasts[0].text != "方案已更新" {
		t.Fatalf("tab=%s toasts=%+v", s.productTab, s.toasts)
	}
}

// 回答问题：整段对话（开场白 + 用户消息）发给引擎，回复的文字、快捷回复、确认卡都写进这段对话。
func TestChatSendStreamsIntoSession(t *testing.T) {
	var got struct {
		Messages []ChatMsg `json:"messages"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		w.Header().Set("Content-Type", "application/x-ndjson")
		lines := []string{
			`{"type":"text","delta":"好，先保存到这里。"}`,
			`{"type":"tool","name":"confirm_answers","args":{"answers":[{"question":"年龄段？","answer":"3–6 岁"}]}}`,
			`{"type":"suggestions","items":["跳过这题"]}`,
			`{"type":"done"}`,
		}
		_, _ = io.WriteString(w, strings.Join(lines, "\n")+"\n")
	}))
	defer srv.Close()

	app.mu.Lock()
	saved := app.engineURL
	app.engineURL = srv.URL
	cs := app.chatFor("answers:p:fp", "answers:p:", "开场白", []string{"帮我想想"})
	app.mu.Unlock()
	defer func() {
		app.mu.Lock()
		app.engineURL = saved
		delete(app.chats, "answers:p:fp")
		app.mu.Unlock()
	}()

	app.chatSend("answers:p:fp", "/api/products/p/chat", "先保存已回答的")
	if len(got.Messages) != 2 || got.Messages[0].Text != "开场白" || got.Messages[1].Text != "先保存已回答的" {
		t.Fatalf("engine should get intro + user message, got %+v", got.Messages)
	}
	if cs.Streaming || len(cs.Msgs) != 2 || cs.Msgs[1].Text != "好，先保存到这里。" {
		t.Fatalf("session = %+v", cs)
	}
	if cs.Card == nil || cs.Card.Name != "confirm_answers" || len(cs.Answers) != 1 || cs.Answers[0].Answer != "3–6 岁" {
		t.Fatalf("confirm card should be filled, card=%+v answers=%+v", cs.Card, cs.Answers)
	}
	if len(cs.chips()) != 0 {
		t.Fatalf("no chips while a card waits, got %v", cs.chips())
	}
}

func TestFormatHelpers(t *testing.T) {
	for in, want := range map[int]string{0: "0", 999: "999", 1364: "1,364", 1500000: "1,500,000"} {
		if got := fmtNum(in); got != want {
			t.Fatalf("fmtNum(%d) = %q, want %q", in, got, want)
		}
	}
	if fingerprint([]string{"a", "b"}) == fingerprint([]string{"a", "c"}) {
		t.Fatal("different question lists should give different fingerprints")
	}
}

// 每个分页都能画出来，不因为缺字段崩掉。
func TestEveryProductTabRenders(t *testing.T) {
	score := 84.0
	n := 2
	app.mu.Lock()
	app.cfgLoaded, app.cfgConfigured = true, true
	app.page, app.selected = PageProducts, "p"
	app.products = []Product{{Slug: "p", Name: "玩具剧场", Version: 2, HasPlan: true, Unknowns: 1}}
	app.prdVer = 2
	app.plan = &PlanView{
		Title: "玩具剧场", OneLiner: "给家长用", Model: "deepseek-v4-flash", Unknowns: []string{"年龄段？"},
		Questions:  []PlanQuestion{{Text: "年龄段？（关系到组件 s09 记忆系统）", Original: "孩子多大", Priority: 1}},
		Facts:      []PlanFact{{Label: "目标", Value: "亲子共演", Status: "明确"}},
		Components: []PlanComponent{{ID: "s01", Name: "Agent Loop", Decision: "必选", Reason: "一切基础"}},
		Layers:     []PlanLayer{{Num: "③", Name: "Agent 编排层", Detail: "s01"}},
		ToolList:   []PlanTool{{Name: "ingest_photo", Permission: "allow", Layer: "①"}},
		Elements:   []PlanElement{{Label: "工具", Items: []string{"识别"}}},
		Review:     &PlanReview{OK: true, Score: &score, Issues: []ReviewIssue{{Section: "a", Problem: "b", Severity: "high"}}},
		Budget:     Usage{Calls: 8, InputTokens: 18380, OutputTokens: 19441},
	}
	app.job = &JobInfo{ID: "1", Kind: "plan", Status: "failed", Error: "API Key 无效", Logs: []string{"  ▶ ① 需求拆解"}}
	app.history = &History{Current: 2, Versions: []HistoryVersion{{Version: 1}, {Version: 2}},
		Log: []HistoryEntry{{Date: "2026-11-21", Version: &n, Title: "新需求", Lines: []string{"新需求：每周小结", "选型变化：s09 二期 → 必选", "人工验证结论：（待填）"}}}}
	app.historyFor = "p:2:0"
	app.mu.Unlock()
	defer func() {
		app.mu.Lock()
		app.selected, app.plan, app.job, app.history, app.products = "", nil, nil, nil, nil
		app.productTab = "overview"
		app.mu.Unlock()
	}()
	tt := ui.NewTester(root, 1280, 820)
	for _, g := range productNav {
		for _, it := range g.items {
			if it.key == "play" {
				continue // 试玩要连引擎拉开场白
			}
			app.mu.Lock()
			app.productTab = it.key
			app.mu.Unlock()
			tt.Frame()
		}
	}
	for _, text := range []string{"版本历史", "审稿意见", "组件选型变了 1 处："} {
		if !tt.HasText(text) {
			t.Fatalf("expected %q on screen", text)
		}
	}
	app.mu.Lock()
	app.selected = ""
	app.mu.Unlock()
	tt.Frame()
	if !tt.HasText("我的产品") {
		t.Fatal("home should list products")
	}
	app.mu.Lock()
	app.page = PageAudits
	app.audits = []AuditSummary{{Slug: "a", Name: "minimal-bot", Finished: true, Completion: 0.38, Mode: "agent"}}
	app.auditSelected = "a"
	app.auditDetail = &AuditDetail{AuditSummary: app.audits[0], Report: "## 一句话判定\n\n**只有雏形。**"}
	app.mu.Unlock()
	tt.Frame()
	if !tt.HasText("只有雏形") || !tt.HasText("重新判断") {
		t.Fatal("audit summary should render")
	}
	app.mu.Lock()
	app.page, app.audits, app.auditSelected, app.auditDetail = PageProducts, nil, "", nil
	app.mu.Unlock()
}

// 引擎的 pending_versions 是 ["V2"] 这样的字符串：解析不能失败，否则加新需求之后产品列表就刷新不了。
func TestProductListParsesPendingVersions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"products":[{"slug":"p","name":"玩具剧场","version":2,"has_plan":true,"unknowns":9,"pending_versions":["V2"],"updated":1.5}]}`)
	}))
	defer srv.Close()
	products, err := listProducts(srv.URL)
	if err != nil || len(products) != 1 || products[0].Version != 2 || products[0].PendingVersions[0] != "V2" {
		t.Fatalf("products=%+v err=%v", products, err)
	}
}
