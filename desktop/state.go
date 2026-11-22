package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Product 是 GET /api/products 里一个产品的摘要（与 blueprint/web/views.py 的 product_summary 对齐）。
type Product struct {
	Slug            string   `json:"slug"`
	Name            string   `json:"name"`
	Version         int      `json:"version"`
	HasPlan         bool     `json:"has_plan"`
	Unknowns        int      `json:"unknowns"`
	ReviewOK        bool     `json:"review_ok"`
	PendingVersions []string `json:"pending_versions"` // 「V2」这样的版本号
	Updated         float64  `json:"updated"`
}

// Page 是内容区的当前页面。
type Page int

const (
	PageProducts Page = iota
	PageNew           // 对话式新建产品
	PageSettings      // 模型设置
	PageAudits        // 外部项目检查
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
	Name     string `json:"name"`
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
	Domain   string `json:"domain"`
	OneLiner string `json:"one_liner"`
	When     string `json:"when"`
}

type PlanLayer struct {
	Num      string `json:"num"` // 引擎给的是圈号「①」…「⑦」，不是数字
	Name     string `json:"name"`
	Question string `json:"question"`
	Detail   string `json:"detail"`
}

// PlanTool 是方案里的一个工具（与 web/src/lib/api.ts 的 Tool 对齐）。
type PlanTool struct {
	Name       string `json:"name"`
	Input      string `json:"input"`
	Output     string `json:"output"`
	SideEffect string `json:"side_effect"`
	Permission string `json:"permission"` // allow | ask | deny
	Layer      string `json:"layer"`
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
	ToolList               []PlanTool        `json:"tool_list"`
	SystemPrompt           string            `json:"system_prompt"`
	ContextMemoryStrategy  string            `json:"context_memory_strategy"`
	SecurityAndDegradation []string          `json:"security_and_degradation"`
	Testing                []string          `json:"testing"`
	Observability          PlanObservability `json:"observability"`
	TechStack              string            `json:"tech_stack_and_deployment"`
	Review                 *PlanReview       `json:"review"`
	DesignAttempts         int               `json:"design_attempts"`
	Budget                 Usage             `json:"budget"`
}

// PlanObservability 方案里的可观测性三项。
type PlanObservability struct {
	Logs       []string `json:"logs"`
	Metrics    []string `json:"metrics"`
	Trajectory string   `json:"trajectory"`
}

// PlanReview 独立审稿结果。
type PlanReview struct {
	OK      bool          `json:"ok"`
	Score   *float64      `json:"score"`
	Summary string        `json:"summary"`
	Issues  []ReviewIssue `json:"issues"`
}

type ReviewIssue struct {
	Section  string `json:"section"`
	Problem  string `json:"problem"`
	Severity string `json:"severity"`
}

// Usage 模型用量（usage.json 的一栏，或方案里的 budget）。
type Usage struct {
	Calls           int `json:"calls"`
	InputTokens     int `json:"input_tokens"`
	OutputTokens    int `json:"output_tokens"`
	CacheReadTokens int `json:"cache_read_tokens"`
}

// ProductUsage 产品累计用量：推导 + 对话。
type ProductUsage struct {
	Total Usage `json:"total"`
	Plan  Usage `json:"plan"`
	Chat  Usage `json:"chat"`
}

// JobInfo 是引擎任务（与 jobs.py 的 to_dict 对齐）。
type JobInfo struct {
	ID       string         `json:"id"`
	Kind     string         `json:"kind"`
	Slug     string         `json:"slug"`
	Status   string         `json:"status"` // running | done | failed
	Logs     []string       `json:"logs"`
	LogCount int            `json:"log_count"`
	Error    string         `json:"error"`
	Started  float64        `json:"started"`
	Finished float64        `json:"finished"`
	Result   map[string]any `json:"result"`
}

// PlayMsg 是试玩页的一行：用户话、Agent 回复或一个模拟动作步骤。
type PlayMsg struct {
	ID       int    // assistant 行的编号，「这里不对」的记录靠它对上号
	Role     string // user / assistant / step
	Text     string
	Step     map[string]string // step 行的动作卡字段
	Feedback bool              // 这条回复下面可以点「这里不对」
}

// AuditSummary 是一个检查结果（与 audits.py 的 audit_summary 对齐）。
type AuditSummary struct {
	Slug       string  `json:"slug"`
	Name       string  `json:"name"`
	Title      string  `json:"title"`
	Root       string  `json:"root"`
	Finished   bool    `json:"finished"`
	Completion float64 `json:"completion"`
	Verdict    string  `json:"verdict"`
	Missing    int     `json:"missing"`
	Weak       int     `json:"weak"`
	Mode       string  `json:"mode"`
	Updated    float64 `json:"updated"`
}

// AuditDetail 是一个检查结果的完整内容（audit_detail）。
type AuditDetail struct {
	AuditSummary
	Path         string `json:"path"`
	Report       string `json:"report"`
	SuggestedPRD string `json:"suggested_prd"`
	ModeNote     string `json:"mode_note"`
	FeaturesFrom string `json:"features_from"`
	FilesSeen    int    `json:"files_seen"`
	Calls        int    `json:"calls"`
}

// AppState 是界面状态。写操作都在后台 goroutine（引擎连接、取数据、流式对话），
// 界面每帧只读；用一把锁串起来，避免数据竞争。
type AppState struct {
	mu sync.Mutex

	page Page

	engineURL    string // 解析到的引擎地址，空 = 还没起来
	engineErr    string // 引擎起不来时的原因（给人看）
	engineStatus string // 启动前准备环境时的进度（左栏底部显示）
	engineCmd    *exec.Cmd
	quitting     bool // 自己要停引擎时置上，退出原因就不当成「引擎退出了」报错

	products []Product
	selected string // 选中产品的 slug
	prdText  string
	prdVer   int
	prdErr   string
	loading  bool

	// 引擎的模型配置状态（左栏、各页的「先配置模型」提示用）
	cfgLoaded     bool
	cfgConfigured bool
	cfgMock       bool
	cfgModel      string

	// 产品详情页
	plan        *PlanView
	planErr     string
	productTab  string // overview answers play revise components architecture prompt review history
	job         *JobInfo
	pollStop    chan struct{}
	usage       ProductUsage
	answerIntro string
	productPath string
	planBusy    bool // 正在发「生成方案 / 继续推导」请求

	// 回答问题、加新需求两段对话，按 key 存（见 chat.go）
	chats map[string]*chatSession

	// 试玩里记下的「这里不对」，按产品存；重新开始不清空
	notes      map[string][]playNote
	noteOpen   int // 正在写批注的回复编号，0 = 没有
	noteDraft  string
	noteGen    int
	reviseSeed map[string]string // 带去加新需求的草稿，按产品存
	playMsgSeq int
	playFP     string                  // 当前试玩开场的指纹（方案变了就换）
	playSaved  map[string]savedProduct // 上次存下、还没接上的试玩，按产品存（见 persist.go）

	// 对话存盘：每个产品文件夹的路径、这次打开后是否已经读回过
	productPaths map[string]string
	restored     map[string]bool
	saveMu       sync.Mutex // 同一时间只写一个文件
	playChips    []string   // 开场的几个场景，第一次回复前显示
	playRole     string

	// 版本历史
	history    *History
	historyErr string
	historyFor string // 拉的是哪个产品、哪个版本，变了就重拉
	prdDoc     *PrdDoc
	prdDocErr  string
	prdDocOpen bool

	// 正在「修改」的那句话：哪段对话、第几条、改成什么
	editKey  string
	editIdx  int
	editText string
	editGen  int

	// 右下角的提示，下一帧由界面线程弹出
	toasts []toastMsg

	// 试玩页
	playIntro     string
	playErr       string
	playStarted   bool
	playMessages  []PlayMsg
	playStreaming bool
	playGen       int // 换产品或发新一句就 +1；旧的流式回复对不上号就丢掉，免得串到别的产品
	playDraft     string
	playInputGen  int  // 试玩输入框已发送的次数，输入框按它重建以清空（见 chatComposer）
	playFeedback  bool // 是否收到过「这里不对」的反馈卡

	// 交接包
	handoffBusy   bool
	handoffStatus string

	// 项目检查页
	audits         []AuditSummary
	auditsErr      string
	auditSelected  string
	auditDetail    *AuditDetail
	auditDetailErr string
	auditJob       *JobInfo
	auditPollStop  chan struct{}
	auditBusy      bool
	auditStatus    string
	auditTab       int  // 0=差距报告 1=本该有的需求文档
	rejudgeOpen    bool // 「重新判断」确认框
	newAuditOpen   bool // 「检查一个项目」对话框
	newAuditPath   string

	// 新建产品对话页
	chatMessages  []ChatMsg
	chatStreaming bool
	chatError     string
	chatConfirm   *ConfirmCard
	chatSuggest   []string
	chatDraft     string
	chatInputGen  int // 新建产品输入框已发送的次数

	// 设置页
	settings    map[string]any
	settingsErr string
	setProvider string
	setModel    string
	setKey      string
	setBaseURL  string
	setStatus   string
}

func newState() *AppState {
	return &AppState{
		productTab: "overview",
		chats:      map[string]*chatSession{},
		notes:      map[string][]playNote{},
		reviseSeed: map[string]string{},
		playSaved:  map[string]savedProduct{},

		productPaths: map[string]string{},
		restored:     map[string]bool{},
	}
}

// toastMsg 一条右下角提示。后台 goroutine 放进队列，界面线程下一帧弹出（见 drainToasts）。
type toastMsg struct {
	text string
	ok   bool
}

// toast 排一条提示。调用方必须已持有 s.mu。
func (s *AppState) toast(text string, ok bool) {
	s.toasts = append(s.toasts, toastMsg{text: text, ok: ok})
}

// loadConfig 读模型是否配好、是不是离线演示：左栏和各页的「先配置模型」提示用。
func (s *AppState) loadConfig() {
	base := s.baseURL()
	if base == "" {
		return
	}
	data, err := getSettings(base)
	if err != nil {
		return
	}
	s.mu.Lock()
	s.cfgLoaded = true
	s.cfgConfigured, _ = data["configured"].(bool)
	s.cfgMock, _ = data["mock"].(bool)
	s.cfgModel, _ = data["model"].(string)
	s.mu.Unlock()
	s.invalidate()
}

func (s *AppState) invalidate() {
	if fn := invalidate.Load(); fn != nil {
		(*fn)()
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
	s.usage, s.answerIntro, s.productPath, s.planBusy = ProductUsage{}, "", "", false
	s.productTab = "overview"
	s.loading = true
	s.playStarted, s.playIntro, s.playMessages, s.playErr, s.playDraft = false, "", nil, "", ""
	s.playStreaming = false
	s.playChips, s.playRole, s.noteOpen, s.playFP = nil, "", 0, ""
	s.playGen++
	s.history, s.historyErr, s.historyFor = nil, "", ""
	s.prdDoc, s.prdDocErr, s.prdDocOpen = nil, "", false
	s.handoffBusy, s.handoffStatus = false, ""
	s.mu.Unlock()
	s.invalidate()
	go s.fetchPRD(slug)
	go s.fetchDetail(slug)
}

func (s *AppState) fetchPRD(slug string) {
	ver, text, err := getPRD(s.baseURL(), slug)
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
	data, err := getProductDetail(s.baseURL(), slug)
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
	s.plan = nil
	if raw, ok := data["plan"]; ok && raw != nil {
		if plan, perr := parsePlan(raw); perr == nil {
			s.plan = plan
			s.planErr = ""
		} else {
			s.planErr = perr.Error()
		}
	}
	if raw, ok := data["usage"]; ok && raw != nil {
		if b, berr := json.Marshal(raw); berr == nil {
			_ = json.Unmarshal(b, &s.usage)
		}
	}
	s.answerIntro, _ = data["answer_intro"].(string)
	s.productPath, _ = data["path"].(string)
	if s.productPath != "" {
		s.productPaths[slug] = s.productPath
		s.restoreProduct(slug) // 第一次打开：把上次存下的对话读回来
	}
	if v, ok := data["version"].(float64); ok {
		s.prdVer = int(v)
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
		failures := 0
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
				if giveUpPolling(&failures, base, s.job) {
					s.mu.Unlock()
					s.invalidate()
					return
				}
				s.mu.Unlock()
				continue // 引擎暂时没响应，下轮再试
			}
			upd, err := parseJob(data)
			if err != nil {
				s.mu.Unlock()
				continue
			}
			failures = 0
			upd.Logs = append(append([]string(nil), job.Logs...), upd.Logs...)
			job = upd // 下一轮在累计后的日志上接，不然中间的增量会丢
			s.job = upd
			if upd.Status != "running" {
				s.onPlanJobFinished(upd)
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

// onPlanJobFinished 推导 / 升版任务跑完：和网页版一样弹提示；加新需求成功就跳到版本历史。
// 调用方必须已持有 s.mu。
func (s *AppState) onPlanJobFinished(job *JobInfo) {
	if job.Status != "done" {
		return
	}
	s.historyFor = "" // 版本历史要重拉
	if job.Kind == "revise" && job.Result != nil {
		newVer := toInt(job.Result["new_version"])
		diff, _ := job.Result["diff"].([]any)
		msg := fmt.Sprintf("已升到 V%d，组件选型变了 %d 处", newVer, len(diff))
		if len(diff) == 0 {
			msg = fmt.Sprintf("已升到 V%d，组件选型没有变化", newVer)
		}
		s.toast(msg, true)
		s.productTab = "history"
		return
	}
	s.toast("方案已更新", true)
}

func toInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}

// startPlan 「生成方案」「继续推导」：POST /plan，之后跟着任务进度走。
func (s *AppState) startPlan(slug string) {
	s.mu.Lock()
	if s.planBusy {
		s.mu.Unlock()
		return
	}
	s.planBusy = true
	base := s.engineURL
	s.mu.Unlock()
	s.invalidate()
	_, err := postJSONData(base, "/api/products/"+url.PathEscape(slug)+"/plan", "{}")
	s.mu.Lock()
	s.planBusy = false
	if err != nil {
		s.toast(err.Error(), false)
	}
	s.mu.Unlock()
	s.fetchDetail(slug)
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
	s.productTab = "play"
	if s.playStarted {
		s.mu.Unlock()
		return
	}
	s.playStarted = true
	s.playErr = ""
	s.mu.Unlock()
	s.invalidate()
	go func() {
		data, err := postEmpty(s.baseURL(), "/api/products/"+url.PathEscape(slug)+"/play-intro")
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.selected != slug {
			return
		}
		if err != nil {
			s.playErr = err.Error()
		} else {
			text, _ := data["text"].(string)
			role, _ := data["role"].(string)
			if role == "" {
				role = "这个产品的使用者"
			}
			if text == "" {
				text = fmt.Sprintf("你现在扮演**%s**。发一句话试试，我会按方案里的设计回应。", role)
			}
			s.playIntro, s.playRole = text, role
			s.playFP, _ = data["fingerprint"].(string)
			s.playChips = nil
			if raw, ok := data["scenarios"].([]any); ok {
				for _, r := range raw {
					if sc, ok := r.(string); ok && sc != "" {
						s.playChips = append(s.playChips, sc)
					}
				}
			}
			s.resumePlay(slug, s.playFP) // 上次没聊完、方案也没变：接着聊
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
	s.playChips = nil // 开场的场景只在第一句之前给
	slug := s.selected
	s.playGen++
	gen := s.playGen
	history := chatFromPlay(s.playMessages) // 在锁里取快照，后台 goroutine 不再碰 s.playMessages
	base := s.engineURL
	s.mu.Unlock()
	s.invalidate()
	go func() {
		err := streamChat(base, "/api/products/"+url.PathEscape(slug)+"/play", history, func(ev ChatEvent) {
			s.mu.Lock()
			if s.playGen != gen { // 用户已经换了产品：这条流的事件不再属于当前界面
				s.mu.Unlock()
				return
			}
			switch ev.Type {
			case "text":
				if n := len(s.playMessages); n == 0 || s.playMessages[n-1].Role != "assistant" {
					s.playMsgSeq++
					s.playMessages = append(s.playMessages, PlayMsg{Role: "assistant", ID: s.playMsgSeq})
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
					if n := len(s.playMessages); n > 0 && s.playMessages[n-1].Role == "assistant" {
						s.playMessages[n-1].Feedback = true
					}
				}
			case "error":
				s.playErr = ev.Message
			}
			s.mu.Unlock()
			s.invalidate()
		})
		s.mu.Lock()
		if s.playGen == gen {
			s.playStreaming = false
			if err != nil {
				s.playErr = err.Error()
			}
		}
		s.mu.Unlock()
		s.invalidate()
		s.saveProduct(slug)
	}()
}

// chatFromPlay 把试玩消息转成引擎要的 {role, text} 列表（step 行不算对话）。
// chatFromPlay 把试玩记录转成发给引擎的历史，和网页版 chat-runtime.tsx 的 toWire 一致：
// 一轮回复里的动作卡（wire 文本）和回复正文按顺序拼成一条 assistant 消息，
// 否则模型下一轮不知道自己已经调过哪些工具。
func chatFromPlay(msgs []PlayMsg) []ChatMsg {
	var out []ChatMsg
	for _, m := range msgs {
		text := m.Text
		if m.Role == "step" {
			text = m.Step["wire"]
		}
		switch {
		case m.Role == "user":
			out = append(out, ChatMsg{Role: "user", Text: text})
		case m.Role != "assistant" && m.Role != "step", text == "":
			continue
		case len(out) > 0 && out[len(out)-1].Role == "assistant":
			out[len(out)-1].Text += "\n" + text
		default:
			out = append(out, ChatMsg{Role: "assistant", Text: text})
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
	base := s.engineURL
	s.mu.Unlock()
	s.invalidate()
	go func() {
		path, err := downloadHandoff(base, slug)
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.selected != slug { // 下载期间换了产品：结果不写到别的产品页上（selectProduct 已复位忙碌状态）
			if err == nil {
				_ = exec.Command("open", "-R", path).Start()
			}
			return
		}
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

// -- 项目检查 ------------------------------------------------------------------

// loadAudits 拉检查结果列表。
func (s *AppState) loadAudits() {
	s.mu.Lock()
	base := s.engineURL
	s.mu.Unlock()
	if base == "" {
		return
	}
	data, err := getAudits(base)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.auditsErr = err.Error()
	} else {
		s.auditsErr = ""
		if raw, ok := data["audits"]; ok {
			if b, err := json.Marshal(raw); err == nil {
				_ = json.Unmarshal(b, &s.audits)
			}
		}
	}
	s.invalidate()
}

// selectAudit 选中一个检查结果：拉报告全文；任务在跑就轮询。
func (s *AppState) selectAudit(slug string) {
	s.mu.Lock()
	if s.auditPollStop != nil {
		close(s.auditPollStop)
		s.auditPollStop = nil
	}
	s.auditSelected = slug
	s.auditDetail, s.auditDetailErr, s.auditJob = nil, "", nil
	s.auditTab = 0
	s.mu.Unlock()
	s.invalidate()
	go s.fetchAudit(slug)
}

func (s *AppState) fetchAudit(slug string) {
	data, err := getAuditDetail(s.baseURL(), slug)
	s.mu.Lock()
	if s.auditSelected != slug {
		s.mu.Unlock()
		return
	}
	if err != nil {
		s.auditDetailErr = fmt.Sprintf("读检查结果失败：%v", err)
		s.mu.Unlock()
		s.invalidate()
		return
	}
	if b, berr := json.Marshal(data); berr == nil {
		var d AuditDetail
		if json.Unmarshal(b, &d) == nil {
			s.auditDetail = &d
			s.auditDetailErr = ""
		}
	}
	if raw, ok := data["job"]; ok && raw != nil {
		if job, jerr := parseJob(raw); jerr == nil {
			s.auditJob = job
			if job.Status == "running" {
				s.startAuditPoll(slug, job)
			}
		}
	}
	s.mu.Unlock()
	s.invalidate()
}

// pickAuditFolder 「选择文件夹…」：弹系统的选文件夹窗口，选中的路径填进对话框。
func (s *AppState) pickAuditFolder() {
	s.mu.Lock()
	if s.auditBusy {
		s.mu.Unlock()
		return
	}
	s.auditBusy, s.auditStatus = true, "在弹出的窗口里选…"
	s.mu.Unlock()
	s.invalidate()
	folder, err := pickFolder(s.baseURL())
	s.mu.Lock()
	s.auditBusy, s.auditStatus = false, ""
	if err != nil {
		s.auditStatus = err.Error()
	} else if folder != "" {
		s.newAuditPath = folder
	}
	s.mu.Unlock()
	s.invalidate()
}

// startAudit 「开始检查」：对填好的路径创建检查，选中它。
func (s *AppState) startAudit() {
	s.mu.Lock()
	path := strings.TrimSpace(s.newAuditPath)
	if s.auditBusy || path == "" {
		s.mu.Unlock()
		return
	}
	s.auditBusy, s.auditStatus = true, "正在开始…"
	s.mu.Unlock()
	s.invalidate()
	data, err := createAudit(s.baseURL(), path)
	s.mu.Lock()
	s.auditBusy, s.auditStatus = false, ""
	if err != nil {
		s.auditStatus = err.Error()
		s.mu.Unlock()
		s.invalidate()
		return
	}
	slug, _ := data["slug"].(string)
	s.newAuditOpen, s.newAuditPath = false, ""
	// loadAudits / selectAudit 自己会加锁，这里必须先放锁，否则整个界面死锁。
	s.mu.Unlock()
	s.loadAudits()
	if slug != "" {
		s.selectAudit(slug)
	}
}

// runAudit 再跑一次检查：recheck 沿用上次「该有什么」的判断，rejudge 从头再判断，resume 接着中断的地方跑。
func (s *AppState) runAudit(slug, mode string) {
	s.mu.Lock()
	if s.auditBusy {
		s.mu.Unlock()
		return
	}
	s.auditBusy = true
	s.auditStatus = ""
	s.mu.Unlock()
	s.invalidate()
	go func() {
		body, _ := json.Marshal(map[string]string{"mode": mode})
		data, err := postJSONData(s.baseURL(), "/api/audits/"+url.PathEscape(slug)+"/run", string(body))
		s.mu.Lock()
		defer s.mu.Unlock()
		s.auditBusy = false
		if err != nil {
			s.toast(err.Error(), false)
			s.invalidate()
			return
		}
		s.auditStatus = ""
		if raw, ok := data["job"]; ok {
			if job, jerr := parseJob(raw); jerr == nil {
				s.auditJob = job
				if job.Status == "running" {
					s.startAuditPoll(slug, job)
				}
			}
		}
		s.invalidate()
	}()
}

// startAuditPoll 检查任务跑完前每 2 秒拉增量日志。调用方必须已持有 s.mu。
func (s *AppState) startAuditPoll(slug string, job *JobInfo) {
	if s.auditPollStop != nil {
		close(s.auditPollStop)
	}
	stop := make(chan struct{})
	s.auditPollStop = stop
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		last := job.LogCount
		failures := 0
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
			if s.auditSelected != slug {
				s.mu.Unlock()
				return
			}
			if err != nil {
				if giveUpPolling(&failures, base, s.auditJob) {
					s.mu.Unlock()
					s.invalidate()
					return
				}
				s.mu.Unlock()
				continue
			}
			upd, err := parseJob(data)
			if err != nil {
				s.mu.Unlock()
				continue
			}
			failures = 0
			upd.Logs = append(append([]string(nil), job.Logs...), upd.Logs...)
			job = upd // 下一轮在累计后的日志上接，不然中间的增量会丢
			s.auditJob = upd
			if upd.Status != "running" {
				if upd.Status == "done" {
					s.toast("检查完成，报告已更新", true)
				}
				s.mu.Unlock()
				s.loadAudits()
				s.fetchAudit(slug)
				return
			}
			last = upd.LogCount
			s.mu.Unlock()
			s.invalidate()
		}
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
	s.mu.Lock()
	base, history := s.engineURL, append([]ChatMsg(nil), s.chatMessages...) // 锁内取快照，确认建产品时会清空它
	s.mu.Unlock()
	err := streamChat(base, "/api/chat/intake", history, func(ev ChatEvent) {
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
		slug, err := createProduct(s.baseURL(), card.Name, card.Idea)
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
		go s.loadConfig()
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

// giveUpPolling 轮询任务进度出错时决定要不要停：引擎已经没了（地址为空），
// 或者连续 15 次（约 30 秒）都拿不到（引擎重启后旧任务号查不到），就停下并在任务上写明原因，
// 不再每 2 秒无休止地重试。调用方必须持有 s.mu。
func giveUpPolling(failures *int, base string, job *JobInfo) bool {
	*failures++
	if base != "" && *failures < 15 {
		return false
	}
	if job != nil && job.Status == "running" {
		job.Status = "failed"
		job.Error = "连不上引擎，查不到这个任务的进度了。引擎重启过的话，刷新一下看结果。"
	}
	return true
}

// baseURL 在锁里读引擎地址：引擎退出的 goroutine 会同时把它清空。
func (s *AppState) baseURL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.engineURL
}
