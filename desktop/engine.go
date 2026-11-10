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
	if _, err := os.Stat(py); err != nil {
		s.mu.Lock()
		s.engineErr = "找不到 " + py + "，先运行 setup.sh 或让引擎装好 .venv"
		s.mu.Unlock()
		s.invalidate()
		return
	}

	args := []string{"-m", "blueprint", "ui", "--no-browser", "--port", "0"}
	if mock {
		args = append(args, "--mock")
	}
	cmd := exec.Command(py, args...)
	cmd.Dir = root
	// stderr 直接透传到终端（app 的 stderr），引擎崩了能看到 traceback。
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		s.failEngine("起引擎失败：" + err.Error())
		return
	}
	s.setProcess(cmd)

	if err := cmd.Start(); err != nil {
		s.failEngine("起引擎失败：" + err.Error())
		return
	}

	// 引擎退出时把原因写进状态（窗口关了、引擎崩了都能在界面上看到）。
	go func() {
		if err := cmd.Wait(); err != nil {
			fmt.Fprintf(os.Stderr, "[desktop] 引擎退出：%v\n", err)
			s.mu.Lock()
			if s.engineURL != "" { // 曾正常连上又退出才报退出原因，别覆盖「起不来」的原始报错
				s.engineErr = "引擎退出了：" + err.Error()
			}
			s.engineURL = ""
			s.mu.Unlock()
			s.invalidate()
		}
	}()

	// 一条管道只有一个读者：用 TeeReader 把引擎日志透传到终端，同时扫端口行。
	lines := make(chan string, 8)
	go func() {
		sc := bufio.NewScanner(io.TeeReader(stdout, os.Stdout))
		for sc.Scan() {
			select {
			case lines <- sc.Text():
			default:
			}
		}
	}()
	deadline := time.After(30 * time.Second)
	var port string
scan:
	for {
		select {
		case line := <-lines:
			if m := startupLine.FindStringSubmatch(line); m != nil {
				port = m[1]
				break scan
			}
		case <-deadline:
			s.failEngine("引擎 30 秒内没起来，看终端里的引擎日志")
			return
		}
	}

	s.mu.Lock()
	s.engineURL = "http://127.0.0.1:" + port
	s.mu.Unlock()
	s.invalidate()
	s.refreshProducts()
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
	s.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

// -- 仓库定位 ----------------------------------------------------------------

// findRepoRoot 从当前目录往上找带 blueprint/ 和 .venv/ 的仓库根。
// 开发时用 `go run ./desktop` 在仓库里跑，二进制分发时放在仓库根附近即可。
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if isRepoRoot(dir) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("没找到引擎目录（blueprint/ 与 .venv/），请把本应用放在 agent-workshop 仓库里运行")
		}
		dir = parent
	}
}

func isRepoRoot(dir string) bool {
	if st, err := os.Stat(filepath.Join(dir, "blueprint", "__main__.py")); err != nil || st.IsDir() {
		return false
	}
	_, err := os.Stat(filepath.Join(dir, ".venv", "bin", "python"))
	return err == nil
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
	path := filepath.Join(outDir, name)
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", err
	}
	return path, nil
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
