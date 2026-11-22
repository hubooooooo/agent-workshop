package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeEngineDir 搭一个最小的引擎目录：mac_env.sh 换成测试用的 prepare_env。
func fakeEngineDir(t *testing.T, prepare string) string {
	t.Helper()
	root := t.TempDir()
	for rel, body := range map[string]string{
		"blueprint/__main__.py": "",
		"requirements.txt":      "",
		"tools/mac_env.sh":      "prepare_env() {\n" + prepare + "\n}\n",
	} {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// 解压出来的网页版包里还没有 .venv：照样认得出是引擎目录（.venv 由应用自己建）。
func TestEngineDirFoundWithoutVenv(t *testing.T) {
	root := fakeEngineDir(t, "return 0")
	if !isRepoRoot(root) {
		t.Fatal("a fresh unzip without .venv should still be recognised")
	}
	if isRepoRoot(filepath.Join(root, "blueprint")) || isRepoRoot(t.TempDir()) {
		t.Fatal("folders without the engine must not match")
	}
}

func TestPrepareEnvReportsProgressAndSuccess(t *testing.T) {
	root := fakeEngineDir(t, `echo "==> 检查 Python 环境 ..."; echo "==> 首次初始化（创建环境，约 1 分钟）..."; return 0`)
	s := newState()
	if !s.prepareEnv(root) {
		t.Fatalf("prepare should succeed, err=%q", s.engineErr)
	}
	if s.engineStatus != "" || s.engineErr != "" {
		t.Fatalf("status should be cleared after success, status=%q err=%q", s.engineStatus, s.engineErr)
	}
}

func TestPrepareEnvWithoutPythonExplainsWhatToDo(t *testing.T) {
	root := fakeEngineDir(t, `echo "检测到这台 Mac 还没安装 Python 3.10+，正在帮你下载安装包（约 40MB）…"; read -r -p "按回车关闭窗口 ..."; return 1`)
	s := newState()
	if s.prepareEnv(root) {
		t.Fatal("prepare must fail when Python is missing")
	}
	if !strings.Contains(s.engineErr, "Python 安装器") || !strings.Contains(s.engineErr, "重新打开本应用") {
		t.Fatalf("message should tell the user to install Python and reopen, got %q", s.engineErr)
	}
}

func TestPrepareEnvFailureShowsTheLastLine(t *testing.T) {
	root := fakeEngineDir(t, `echo "==> 安装依赖（已装好的会跳过）..."; echo "ERROR: Could not find a version that satisfies anthropic"; return 1`)
	s := newState()
	if s.prepareEnv(root) {
		t.Fatal("prepare must fail")
	}
	if !strings.Contains(s.engineErr, "Could not find a version") {
		t.Fatalf("the error should carry the script's last line, got %q", s.engineErr)
	}
}
