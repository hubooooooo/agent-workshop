package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var startupLine = regexp.MustCompile(`界面已启动：http://127\.0\.0\.1:(\d+)/`)

// startEngine 拉起 Python 引擎并等它打印端口；期间把 products 列表也取回来。
// 第一次打开（解压出来还没有 .venv）时先准备 Python 环境，和双击「启动界面.command」走同一套脚本。
func (s *AppState) startEngine(mock bool) {
	root, err := findRepoRoot()
	if err != nil {
		s.mu.Lock()
		s.engineErr = err.Error()
		s.mu.Unlock()
		s.invalidate()
		return
	}
	py := filepath.Join(root, ".venv", "bin", "python")
	prepared := false
	if _, err := os.Stat(py); err != nil {
		if !s.prepareEnv(root) {
			return
		}
		prepared = true
	}
	if s.launchEngine(root, py, mock) || prepared {
		return
	}
	// 引擎一启动就退出，多半是依赖没装全（比如更新后 requirements.txt 多了一项）：补装一遍再试一次。
	s.mu.Lock()
	s.engineErr, s.quitting = "", false
	s.mu.Unlock()
	if s.prepareEnv(root) {
		s.launchEngine(root, py, mock)
	}
}

// prepareEnv 跑 tools/mac_env.sh 的 prepare_env：找 Python 3.10+（没有就下载并打开安装器）、建 .venv、装依赖。
// 进度一行行显示在左栏底部。返回环境是否就绪。
func (s *AppState) prepareEnv(root string) bool {
	s.setEngineStatus("正在准备 Python 环境（第一次大约 1 分钟）…")
	cmd := exec.Command("/bin/bash", "-c", "source tools/mac_env.sh && prepare_env")
	cmd.Dir = root
	cmd.Stdin = nil // 脚本里「按回车关闭」读不到输入，直接往下走
	out, err := cmd.StdoutPipe()
	if err != nil {
		s.failEngine("准备 Python 环境失败：" + err.Error())
		return false
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		s.failEngine("准备 Python 环境失败：" + err.Error())
		return false
	}
	var last string
	noPython := false
	sc := bufio.NewScanner(io.TeeReader(out, os.Stdout))
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "==>"))
		if line == "" {
			continue
		}
		if strings.Contains(line, "还没安装 Python") {
			noPython = true
		}
		last = line
		s.setEngineStatus(line)
	}
	err = cmd.Wait()
	s.setEngineStatus("")
	switch {
	case noPython:
		s.failEngine("这台 Mac 还没有 Python 3.10+：已经帮你下载并打开了 Python 安装器（没弹出来就到 python.org 下载 3.12）。装好后重新打开本应用。")
		return false
	case err != nil:
		s.failEngine("准备 Python 环境失败：" + last)
		return false
	}
	return true
}

func (s *AppState) setEngineStatus(text string) {
	s.mu.Lock()
	s.engineStatus = text
	s.mu.Unlock()
	s.invalidate()
}

// launchEngine 起引擎进程、等它报端口。引擎一启动就退出时返回 false（调用方可以补装依赖再试）。
func (s *AppState) launchEngine(root, py string, mock bool) bool {
	args := []string{"-m", "blueprint", "ui", "--no-browser", "--port", "0"}
	if mock {
		args = append(args, "--mock")
	}
	cmd := exec.Command(py, args...)
	cmd.Dir = root
	// stdout 接的是管道，Python 默认整块缓冲，「界面已启动」那行会一直攒着不发，
	// 等满 30 秒就被当成没起来。关掉缓冲让启动行立刻送到。
	cmd.Env = append(os.Environ(), "PYTHONUNBUFFERED=1")
	// stderr 直接透传到终端（app 的 stderr），引擎崩了能看到 traceback。
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		s.failEngine("起引擎失败：" + err.Error())
		return true
	}
	s.setProcess(cmd)

	if err := cmd.Start(); err != nil {
		s.failEngine("起引擎失败：" + err.Error())
		return true
	}

	// 引擎退出时把原因写进状态（窗口关了、引擎崩了都能在界面上看到）。
	go func() {
		err := cmd.Wait()
		reason := "正常退出"
		if err != nil {
			reason = err.Error()
		}
		fmt.Fprintf(os.Stderr, "[desktop] 引擎退出：%s\n", reason)
		s.mu.Lock()
		// 退出码 0 也算退出（之前只认报错，界面仍显示已连接）。
		// 曾正常连上又退出才报退出原因，别覆盖「起不来」的原始报错。
		if s.engineURL != "" && !s.quitting {
			s.engineErr = "引擎退出了：" + reason
		}
		s.engineURL = ""
		s.mu.Unlock()
		s.invalidate()
	}()

	// 一条管道只有一个读者：用 TeeReader 把引擎日志透传到终端，同时扫端口行。
	lines := make(chan string, 8)
	go func() {
		defer close(lines) // 管道读到头（引擎退出）就告诉下面别再等满 30 秒
		sc := bufio.NewScanner(io.TeeReader(stdout, os.Stdout))
		sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
		for sc.Scan() {
			select {
			case lines <- sc.Text():
			default:
			}
		}
		// 超长的一行会让 Scanner 停下；剩下的照样读走，否则管道写满，引擎的 print 会卡住
		_, _ = io.Copy(os.Stdout, stdout)
	}()
	deadline := time.After(30 * time.Second)
	var port string
scan:
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				s.failEngine("引擎刚启动就退出了，看终端里的报错（常见原因：依赖没装好、.venv 坏了）")
				return false
			}
			if m := startupLine.FindStringSubmatch(line); m != nil {
				port = m[1]
				break scan
			}
		case <-deadline:
			s.failEngine("引擎 30 秒内没起来，看终端里的引擎日志")
			return true
		}
	}

	s.mu.Lock()
	s.engineURL = "http://127.0.0.1:" + port
	s.mu.Unlock()
	s.invalidate()
	s.refreshProducts()
	s.loadConfig()
	return true
}

func (s *AppState) failEngine(msg string) {
	s.mu.Lock()
	s.engineErr = msg
	s.mu.Unlock()
	s.stopEngine()
	s.invalidate()
}

// refreshProducts 拉产品列表；引擎每次重启后或点刷新时调用。
func (s *AppState) refreshProducts() {
	s.mu.Lock()
	base := s.engineURL
	s.mu.Unlock()
	if base == "" {
		return
	}
	products, err := listProducts(base)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.engineErr = "读产品列表失败：" + err.Error()
	} else {
		s.products = products
		s.engineErr = ""
	}
	s.invalidate()
}

// -- 引擎进程管理 ------------------------------------------------------------

func (s *AppState) setProcess(cmd *exec.Cmd) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.engineCmd = cmd
}

func (s *AppState) stopEngine() {
	s.mu.Lock()
	cmd := s.engineCmd
	s.quitting = true
	s.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

// -- 仓库定位 ----------------------------------------------------------------

// findRepoRoot 先按应用自身所在位置（打包成 .app 后从 Finder 启动时 cwd 是 /），
// 再按当前目录，向上找引擎目录（带 blueprint/ 和 tools/mac_env.sh；.venv 没有的话启动时会自己建）。
func findRepoRoot() (string, error) {
	starts := []string{}
	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			starts = append(starts, filepath.Dir(resolved))
		}
	}
	if wd, err := os.Getwd(); err == nil {
		starts = append(starts, wd)
	}
	for _, start := range starts {
		dir := start
		for {
			if isRepoRoot(dir) {
				return dir, nil
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return "", fmt.Errorf("没找到引擎：本应用要放在解压出来的 agent-workshop 文件夹里（和「启动界面.command」放在一起）才能用")
}

func isRepoRoot(dir string) bool {
	for _, rel := range []string{"blueprint/__main__.py", "tools/mac_env.sh", "requirements.txt"} {
		if st, err := os.Stat(filepath.Join(dir, rel)); err != nil || st.IsDir() {
			return false
		}
	}
	return true
}

// -- HTTP 客户端 -------------------------------------------------------------

// apiGet 走引擎的 JSON 接口。引擎校验 Host 头（127.0.0.1:port），
// Go 客户端天然带上正确 Host；不发 Origin，引擎放行。
func apiGet(base, path string) (map[string]any, error) {
	req, err := http.NewRequest(http.MethodGet, base+path, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("连不上引擎 %s：%w", base, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("引擎返回的不是 JSON：%s", truncate(string(body), 120))
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%s", data["error"])
	}
	return data, nil
}

func listProducts(base string) ([]Product, error) {
	data, err := apiGet(base, "/api/products")
	if err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(data["products"])
	var products []Product
	if err := json.Unmarshal(raw, &products); err != nil {
		return nil, fmt.Errorf("产品列表格式不对：%w", err)
	}
	return products, nil
}

func getPRD(base, slug string) (int, string, error) {
	data, err := apiGet(base, "/api/products/"+url.PathEscape(slug)+"/prd")
	if err != nil {
		return 0, "", err
	}
	text, _ := data["text"].(string)
	ver, _ := data["version"].(float64)
	return int(ver), text, nil
}

// postJSON 走引擎的 POST 接口（引擎要求 Content-Type: application/json）。
func postJSON(base, path, body string) error {
	req, err := http.NewRequest(http.MethodPost, base+path, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("连不上引擎 %s：%w", base, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("引擎报错：%s", truncate(string(data), 200))
	}
	return nil
}

// ChatEvent 是引擎逐行推送的 NDJSON 事件（与 web/src/lib/api.ts 的 ChatEvent 对齐）。
type ChatEvent struct {
	Type    string         `json:"type"`
	Delta   string         `json:"delta"`
	Name    string         `json:"name"`
	Args    map[string]any `json:"args"`
	Data    map[string]any `json:"data"`
	Items   []string       `json:"items"`
	Message string         `json:"message"`
}

// streamChat 走引擎的流式对话接口，每收到一个事件调一次 onEvent。
// messages 是完整历史（含刚发的这条），引擎自己拼上下文。
func streamChat(base, path string, messages []ChatMsg, onEvent func(ChatEvent)) error {
	raw, _ := json.Marshal(map[string]any{"messages": messages})
	req, err := http.NewRequest(http.MethodPost, base+path, strings.NewReader(string(raw)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 0} // 流式接口不设总超时，靠引擎自己结束
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("连不上引擎：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		var data map[string]any
		_ = json.Unmarshal(body, &data)
		return fmt.Errorf("%v", data["error"])
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20) // 单行可能是一整段回复
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var ev ChatEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue // 半行/杂音跳过，不打断整条流
		}
		if ev.Type == "done" {
			return nil
		}
		if ev.Type == "error" {
			onEvent(ev)
			return fmt.Errorf("%s", ev.Message)
		}
		onEvent(ev)
	}
	return sc.Err()
}

// createProduct 确认卡片后真正建产品，返回 slug。
func createProduct(base, name, idea string) (string, error) {
	raw, _ := json.Marshal(map[string]string{"name": name, "idea": idea})
	data, err := postJSONData(base, "/api/products", string(raw))
	if err != nil {
		return "", err
	}
	slug, _ := data["slug"].(string)
	if slug == "" {
		return "", fmt.Errorf("引擎没返回产品标识")
	}
	return slug, nil
}

// getSettings 读模型设置（settings() 的原始 dict）。
func getSettings(base string) (map[string]any, error) {
	return apiGet(base, "/api/settings")
}

// getProductDetail 读产品详情（含结构化方案 plan_view 和最新任务 job）。
func getProductDetail(base, slug string) (map[string]any, error) {
	return apiGet(base, "/api/products/"+url.PathEscape(slug))
}

// getAudits 拉检查结果列表。
func getAudits(base string) (map[string]any, error) {
	return apiGet(base, "/api/audits")
}

// getAuditDetail 读一个检查结果的完整内容。
func getAuditDetail(base, slug string) (map[string]any, error) {
	return apiGet(base, "/api/audits/"+url.PathEscape(slug))
}

// pickFolder 让引擎弹 macOS 选文件夹窗口，返回路径（取消返回空串）。
func pickFolder(base string) (string, error) {
	data, err := postJSONData(base, "/api/audits/pick-folder", "{}")
	if err != nil {
		return "", err
	}
	path, _ := data["path"].(string)
	return path, nil
}

// createAudit 对一个项目目录创建检查。
func createAudit(base, path string) (map[string]any, error) {
	body, _ := json.Marshal(map[string]string{"path": path})
	return postJSONData(base, "/api/audits", string(body))
}

// getJobUpdates 拉任务的增量日志（?since=N），返回任务的完整 dict。
func getJobUpdates(base, id string, since int) (map[string]any, error) {
	return apiGet(base, fmt.Sprintf("/api/jobs/%s?since=%d", url.PathEscape(id), since))
}

// postEmpty 走引擎的无参 POST 接口（play-intro 这类）。
func postEmpty(base, path string) (map[string]any, error) {
	return postJSONData(base, path, "{}")
}

// downloadHandoff 下载交接包 zip，存到用户的下载目录，返回落盘路径。
func downloadHandoff(base, slug string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, base+"/api/products/"+url.PathEscape(slug)+"/handoff", nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("连不上引擎：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		var data map[string]any
		_ = json.Unmarshal(body, &data)
		return "", fmt.Errorf("%v", data["error"])
	}
	name := slug + "-交接包.zip"
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if m := filenameStar.FindStringSubmatch(cd); m != nil {
			if decoded, err := url.PathUnescape(m[1]); err == nil {
				name = decoded
			}
		}
	}
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	outDir := filepath.Join(dir, "Downloads")
	if st, err := os.Stat(outDir); err != nil || !st.IsDir() {
		outDir = dir
	}
	// 先写到临时文件，完整收到再改名：中途失败不会留下半个 zip，也不会覆盖已有的同名文件。
	name = filepath.Base(name) // 文件名来自响应头，不许带目录
	tmp, err := os.CreateTemp(outDir, ".agent-handoff-*.part")
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", fmt.Errorf("下载中断：%w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	path := uniquePath(outDir, name)
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return path, nil
}

// uniquePath 在 dir 里给 name 找一个还没被占用的路径：「x.zip」被占就用「x (2).zip」…
func uniquePath(dir, name string) string {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	path := filepath.Join(dir, name)
	for i := 2; ; i++ {
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			return path
		}
		path = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, i, ext))
	}
}

var filenameStar = regexp.MustCompile(`filename\*=UTF-8''([^;]+)`)

// saveSettingsJSON 保存模型设置，返回引擎的 {ok, message}。
func saveSettingsJSON(base, body string) (map[string]any, error) {
	return postJSONData(base, "/api/settings", body)
}

// postJSONData 是 postJSON 的带返回值版本。
func postJSONData(base, path, body string) (map[string]any, error) {
	req, err := http.NewRequest(http.MethodPost, base+path, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("连不上引擎 %s：%w", base, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var parsed map[string]any
	if len(data) > 0 {
		if err := json.Unmarshal(data, &parsed); err != nil {
			return nil, fmt.Errorf("引擎返回的不是 JSON：%s", truncate(string(data), 120))
		}
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%v", parsed["error"])
	}
	return parsed, nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return strings.TrimSpace(s)
}
