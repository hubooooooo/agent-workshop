package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"sync"
	"time"
)

// Product 是 GET /api/products 里一个产品的摘要（与 blueprint/web/views.py 的 product_summary 对齐）。
type Product struct {
	Slug            string  `json:"slug"`
	Name            string  `json:"name"`
	Version         int     `json:"version"`
	HasPlan         bool    `json:"has_plan"`
	Unknowns        int     `json:"unknowns"`
	ReviewOK        bool    `json:"review_ok"`
	PendingVersions []int   `json:"pending_versions"`
	Updated         float64 `json:"updated"`
}

// Page 是内容区的当前页面。
type Page int

const (
	PageProducts Page = iota
	PageNew      // 对话式新建产品
	PageSettings // 模型设置
)

// ChatMsg 是一条对话消息（与引擎 chat.parse_messages 的格式一致）。
type ChatMsg struct {
	Role string `json:"role"` // user / assistant
	Text string `json:"text"`
}

// ConfirmCard 是引擎 confirm_product 工具弹出的待确认卡片。
type ConfirmCard struct {
	Name string `json:"name"`
	Idea string `json:"idea"`
}

// -- 方案页数据结构（与 blueprint/web/views.py 的 plan_view 对齐）------------

type PlanQuestion struct {
	Text     string `json:"text"`
	Original string `json:"original"`
	Priority int    `json:"priority"`
}

type PlanComponent struct {
	ID       string `json:"id"`
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
	Domain   string `json:"domain"`
	OneLiner string `json:"one_liner"`
	When     string `json:"when"`
}

type PlanLayer struct {
	Num      int    `json:"num"`
	Name     string `json:"name"`
	Question string `json:"question"`
	Detail   string `json:"detail"`
}

type PlanFact struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Value  string `json:"value"`
	Status string `json:"status"`
}

type PlanElement struct {
	Key   string   `json:"key"`
	Label string   `json:"label"`
	Items []string `json:"items"`
}

type PlanView struct {
	Title                  string            `json:"title"`
	OneLiner               string            `json:"one_liner"`
	Runner                 string            `json:"runner"`
	Model                  string            `json:"model"`
	Unknowns               []string          `json:"unknowns"`
	Questions              []PlanQuestion    `json:"questions"`
	DevItems               []PlanQuestion    `json:"dev_items"`
	Facts                  []PlanFact        `json:"facts"`
	Elements               []PlanElement     `json:"elements"`
	Components             []PlanComponent   `json:"components"`
	Layers                 []PlanLayer       `json:"layers"`
	ToolList               []string          `json:"tool_list"`
	SystemPrompt           string            `json:"system_prompt"`
	ContextMemoryStrategy  string            `json:"context_memory_strategy"`
	SecurityAndDegradation []string          `json:"security_and_degradation"`
	Testing                []string          `json:"testing"`
	Review                 map[string]any    `json:"review"`
}

// JobInfo 是引擎任务（与 jobs.py 的 to_dict 对齐）。
type JobInfo struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	Slug      string   `json:"slug"`
	Status    string   `json:"status"` // running | done | failed
	Logs      []string `json:"logs"`
	LogCount  int      `json:"log_count"`
	Error     string   `json:"error"`
	Started   float64  `json:"started"`
	Finished  float64  `json:"finished"`
}

// PlayMsg 是试玩页的一行：用户话、Agent 回复或一个模拟动作步骤。
type PlayMsg struct {
	Role string            // user / assistant / step
	Text string
	Step map[string]string // step 行的动作卡字段
}

// AppState 是界面状态。写操作都在后台 goroutine（引擎连接、取数据、流式对话），
// 界面每帧只读；用一把锁串起来，避免数据竞争。
type AppState struct {
	mu sync.Mutex

	page Page

	engineURL string // 解析到的引擎地址，空 = 还没起来
	engineErr string // 引擎起不来时的原因（给人看）
	engineCmd *exec.Cmd

	products []Product
	selected string // 选中产品的 slug
	prdText  string
	prdVer   int
	prdErr   string
	loading  bool

	// 产品详情页（方案）
	plan       *PlanView
	planErr    string
	productTab int // 0=PRD 1=方案 2=试玩
	job        *JobInfo
	pollStop   chan struct{}

	// 试玩页
	playIntro     string
	playErr       string
	playStarted   bool
	playMessages  []PlayMsg
	playStreaming bool
	playDraft     string
	playFeedback  bool // 是否收到过「这里不对」的反馈卡

	// 交接包
	handoffBusy  bool
	handoffStatus string

	// 新建产品对话页
	chatMessages  []ChatMsg
	chatStreaming bool
	chatError     string
	chatConfirm   *ConfirmCard
	chatSuggest   []string
	chatDraft     string

	// 设置页
	settings    map[string]any
	settingsErr string
	setProvider string
	setModel    string
	setKey      string
	setBaseURL  string
	setStatus   string
}

func newState() *AppState { return &AppState{} }

func (s *AppState) invalidate() {
	if invalidate != nil {
		invalidate()
	}
}

// -- 产品选择 ---------------------------------------------------------------

// selectProduct 切换选中的产品，异步拉 PRD 全文和结构化方案。
func (s *AppState) selectProduct(slug string) {
	s.mu.Lock()
	if s.pollStop != nil {
		close(s.pollStop)
		s.pollStop = nil
	}
	s.selected = slug
	s.page = PageProducts
	s.prdText, s.prdVer, s.prdErr = "", 0, ""
	s.plan, s.job, s.planErr = nil, nil, ""
	s.productTab = 0
	s.loading = true
	s.playStarted, s.playIntro, s.playMessages, s.playErr, s.playDraft = false, "", nil, "", ""
	s.playStreaming = false
	s.handoffBusy, s.handoffStatus = false, ""
	s.mu.Unlock()
	s.invalidate()
	go s.fetchPRD(slug)
	go s.fetchDetail(slug)
}

func (s *AppState) fetchPRD(slug string) {
	ver, text, err := getPRD(s.engineURL, slug)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.selected != slug {
		return // 用户已经选了别的产品，丢弃过期结果
	}
	s.loading = false
	if err != nil {
		s.prdErr = fmt.Sprintf("读 PRD 失败：%v", err)
	} else {
		s.prdVer, s.prdText = ver, text
	}
	s.invalidate()
}

// fetchDetail 拉结构化方案 + 最新任务；任务在跑就接着轮询进度。
func (s *AppState) fetchDetail(slug string) {
	data, err := getProductDetail(s.engineURL, slug)
	s.mu.Lock()
	if s.selected != slug {
		s.mu.Unlock()
		return
	}
	if err != nil {
		s.planErr = fmt.Sprintf("读方案失败：%v", err)
		s.mu.Unlock()
		s.invalidate()
		return
	}
	if raw, ok := data["plan"]; ok && raw != nil {
		if plan, perr := parsePlan(raw); perr == nil {
			s.plan = plan
			s.planErr = ""
		} else {
			s.planErr = perr.Error()
		}
	}
	if raw, ok := data["job"]; ok && raw != nil {
		if job, jerr := parseJob(raw); jerr == nil {
			s.job = job
			if job.Status == "running" {
				s.startPoll(slug, job)
			}
		}
	}
	s.mu.Unlock()
	s.invalidate()
}

// startPoll 在任务跑完前每 2 秒拉一次增量日志。调用方必须已持有 s.mu。
func (s *AppState) startPoll(slug string, job *JobInfo) {
	if s.pollStop != nil {
		close(s.pollStop)
	}
	stop := make(chan struct{})
	s.pollStop = stop
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		last := job.LogCount
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
			}
			s.mu.Lock()
			base := s.engineURL
			s.mu.Unlock()
			data, err := getJobUpdates(base, job.ID, last)
			s.mu.Lock()
			if s.selected != slug {
				s.mu.Unlock()
				return
			}
			if err != nil {
				s.mu.Unlock()
				continue // 引擎暂时没响应，下轮再试
			}
			upd, err := parseJob(data)
			if err != nil {
				s.mu.Unlock()
				continue
			}
			upd.Logs = append(append([]string(nil), job.Logs...), upd.Logs...)
			s.job = upd
			if upd.Status != "running" {
				s.mu.Unlock()
				s.refreshProducts() // 推导完成：产品摘要变了（待确认数、审稿状态）
				s.fetchDetail(slug)
				return
			}
			last = upd.LogCount
			s.mu.Unlock()
			s.invalidate()
		}
	}()
}

func parsePlan(raw any) (*PlanView, error) {
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var p PlanView
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func parseJob(raw any) (*JobInfo, error) {
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var j JobInfo
	if err := json.Unmarshal(b, &j); err != nil {
		return nil, err
	}
	return &j, nil
}

// -- 试玩 --------------------------------------------------------------------

// startPlay 进入试玩：拉开场白（角色说明），没方案的报错。
func (s *AppState) startPlay(slug string) {
	s.mu.Lock()
	s.productTab = 2
	if s.playStarted {
		s.mu.Unlock()
		return
	}
	s.playStarted = true
	s.playErr = ""
	s.mu.Unlock()
	s.invalidate()
	go func() {
		data, err := postEmpty(s.engineURL, "/api/products/"+url.PathEscape(slug)+"/play-intro")
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.selected != slug {
			return
		}
		if err != nil {
			s.playErr = err.Error()
		} else {
			text, _ := data["text"].(string)
			if text == "" {
				role, _ := data["role"].(string)
				text = fmt.Sprintf("你现在扮演**%s**。发一句话试试，我会按方案里的设计回应。", role)
			}
			s.playIntro = text
		}
		s.invalidate()
	}()
}

// sendPlay 发一句试玩的话，流式收 Agent 的模拟动作和回复。
func (s *AppState) sendPlay(text string) {
	s.mu.Lock()
	if s.playStreaming || text == "" {
		s.mu.Unlock()
		return
	}
	s.playMessages = append(s.playMessages, PlayMsg{Role: "user", Text: text})
	s.playStreaming = true
	s.playErr = ""
	s.playDraft = ""
	s.playFeedback = false
	slug := s.selected
	s.mu.Unlock()
	s.invalidate()
	go func() {
		err := streamChat(s.engineURL, "/api/products/"+url.PathEscape(slug)+"/play", chatFromPlay(s.playMessages), func(ev ChatEvent) {
			s.mu.Lock()
			switch ev.Type {
			case "text":
				if n := len(s.playMessages); n == 0 || s.playMessages[n-1].Role != "assistant" {
					s.playMessages = append(s.playMessages, PlayMsg{Role: "assistant"})
				}
				s.playMessages[len(s.playMessages)-1].Text += ev.Delta
			case "data":
				switch ev.Name {
				case "play_step":
					step := map[string]string{}
					for k, v := range ev.Data {
						if sv, ok := v.(string); ok {
							step[k] = sv
						}
					}
					s.playMessages = append(s.playMessages, PlayMsg{Role: "step", Step: step})
				case "play_feedback":
					s.playFeedback = true
				}
			case "error":
				s.playErr = ev.Message
			}
			s.mu.Unlock()
			s.invalidate()
		})
		s.mu.Lock()
		s.playStreaming = false
		if err != nil {
			s.playErr = err.Error()
		}
		s.mu.Unlock()
		s.invalidate()
	}()
}

// chatFromPlay 把试玩消息转成引擎要的 {role, text} 列表（step 行不算对话）。
func chatFromPlay(msgs []PlayMsg) []ChatMsg {
	var out []ChatMsg
	for _, m := range msgs {
		if m.Role == "user" || m.Role == "assistant" {
			out = append(out, ChatMsg{Role: m.Role, Text: m.Text})
		}
	}
	return out
}

// -- 交接包 ------------------------------------------------------------------

// downloadHandoffZip 下载交接包并打开所在目录。
func (s *AppState) downloadHandoffZip(slug string) {
	s.mu.Lock()
	if s.handoffBusy {
		s.mu.Unlock()
		return
	}
	s.handoffBusy = true
	s.handoffStatus = ""
	s.mu.Unlock()
	s.invalidate()
	go func() {
		path, err := downloadHandoff(s.engineURL, slug)
		s.mu.Lock()
		defer s.mu.Unlock()
		s.handoffBusy = false
		if err != nil {
			s.handoffStatus = "下载失败：" + err.Error()
		} else {
			_ = exec.Command("open", "-R", path).Start() // Finder 里高亮这个文件
			s.handoffStatus = "已下载：" + path
		}
		s.invalidate()
	}()
}

// -- 新建产品对话 -----------------------------------------------------------

// sendChat 把用户输入发给引擎的追问流，事件逐条进状态。
func (s *AppState) sendChat(text string) {
	s.mu.Lock()
	if s.chatStreaming || text == "" {
		s.mu.Unlock()
		return
	}
	s.chatMessages = append(s.chatMessages, ChatMsg{Role: "user", Text: text})
	s.chatStreaming = true
	s.chatError = ""
	s.chatDraft = ""
	s.chatSuggest = nil
	s.mu.Unlock()
	s.invalidate()
	go s.streamIntake()
}

// streamIntake 消费 NDJSON 事件流：text 增量拼进回复，tool 弹确认卡，suggestions 变成快捷按钮。
func (s *AppState) streamIntake() {
	err := streamChat(s.engineURL, "/api/chat/intake", s.chatMessages, func(ev ChatEvent) {
		s.mu.Lock()
		switch ev.Type {
		case "text":
			if n := len(s.chatMessages); n == 0 || s.chatMessages[n-1].Role != "assistant" {
				s.chatMessages = append(s.chatMessages, ChatMsg{Role: "assistant"})
			}
			s.chatMessages[len(s.chatMessages)-1].Text += ev.Delta
		case "tool":
			if ev.Name == "confirm_product" {
				raw, _ := json.Marshal(ev.Args)
				var card ConfirmCard
				_ = json.Unmarshal(raw, &card)
				s.chatConfirm = &card
			}
		case "suggestions":
			s.chatSuggest = ev.Items
		case "error":
			s.chatError = ev.Message
		}
		s.mu.Unlock()
		s.invalidate()
	})
	s.mu.Lock()
	s.chatStreaming = false
	if err != nil {
		s.chatError = err.Error()
	}
	s.mu.Unlock()
	s.invalidate()
}

// confirmProduct 用户点了确认卡片：真正创建产品并跳到产品页。
func (s *AppState) confirmProduct() {
	s.mu.Lock()
	card := s.chatConfirm
	if card == nil {
		s.mu.Unlock()
		return
	}
	s.chatConfirm = nil
	s.chatSuggest = nil
	s.mu.Unlock()
	go func() {
		slug, err := createProduct(s.engineURL, card.Name, card.Idea)
		s.mu.Lock()
		if err != nil {
			s.chatError = fmt.Sprintf("创建产品失败：%v", err)
			s.mu.Unlock()
			s.invalidate()
			return
		}
		// 成功后回到产品页：推导任务在引擎后台跑，先刷新列表再选中新产品。
		s.chatMessages = nil
		s.mu.Unlock()
		s.refreshProducts()
		s.selectProduct(slug)
	}()
}

// -- 设置页 ------------------------------------------------------------------

// loadSettings 拉模型设置并填进表单。
func (s *AppState) loadSettings() {
	s.mu.Lock()
	base := s.engineURL
	s.mu.Unlock()
	if base == "" {
		return
	}
	data, err := getSettings(base)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.settingsErr = err.Error()
	} else {
		s.settings = data
		s.settingsErr = ""
		s.setProvider, _ = data["provider"].(string)
		s.setModel, _ = data["model"].(string)
		s.setBaseURL, _ = data["base_url"].(string)
		s.setKey = "" // Key 不回显，重新粘贴才覆盖
	}
	s.invalidate()
}

func (s *AppState) saveSettings() {
	s.mu.Lock()
	body := map[string]string{
		"provider": s.setProvider,
		"model":    s.setModel,
		"api_key":  s.setKey,
		"base_url": s.setBaseURL,
	}
	base := s.engineURL
	s.mu.Unlock()
	raw, _ := json.Marshal(body)
	result, err := saveSettingsJSON(base, string(raw))
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.setStatus = "保存失败：" + err.Error()
		s.invalidate()
		return
	}
	if ok, _ := result["ok"].(bool); ok {
		s.setStatus = "已保存：" + toString(result["message"])
		s.setKey = ""
	} else {
		s.setStatus = toString(result["message"])
	}
	s.invalidate()
}

func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}
