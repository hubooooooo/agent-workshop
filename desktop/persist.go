package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// 回答问题、加新需求、试玩的对话，以及「这里不对」的记录，存在这台电脑上：关掉应用再打开还能接着来。
// 和网页版存在浏览器里一样：问题清单变了（重新推导）、版本升了、方案变了，旧的对话就作废；
// 对话确认完成后删掉。存在应用自己的目录里，不放进产品文件夹，免得跟着交接包发出去。

// savedChat 一段对话里要留下来的部分。
type savedChat struct {
	Intro       string    `json:"intro"`
	Msgs        []ChatMsg `json:"msgs"`
	OpenChips   []string  `json:"open_chips,omitempty"`
	Chips       []string  `json:"chips,omitempty"`
	Replied     bool      `json:"replied,omitempty"`
	Card        *toolCard `json:"card,omitempty"`
	Answers     []qaPair  `json:"answers,omitempty"`
	Title       string    `json:"title,omitempty"`
	Requirement string    `json:"requirement,omitempty"`
}

// savedProduct 一个产品存下来的全部对话。
type savedProduct struct {
	Chats   map[string]savedChat `json:"chats,omitempty"`
	PlayFP  string               `json:"play_fingerprint,omitempty"` // 试玩开场的指纹：方案变了就对不上，不再恢复
	Play    []PlayMsg            `json:"play,omitempty"`
	PlaySeq int                  `json:"play_seq,omitempty"`
	Notes   []playNote           `json:"notes,omitempty"`
}

func (p savedProduct) empty() bool {
	return len(p.Chats) == 0 && len(p.Play) == 0 && len(p.Notes) == 0
}

// chatStoreDir 存对话的目录；测试里换成临时目录。
var chatStoreDir = func() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "Agent 工坊", "chats")
}

// storeFile 按产品文件夹的完整路径起文件名：两个仓库里同名的产品不会串。
func storeFile(productPath string) string {
	dir := chatStoreDir()
	if dir == "" || productPath == "" {
		return ""
	}
	sum := sha1.Sum([]byte(productPath))
	return filepath.Join(dir, hex.EncodeToString(sum[:8])+".json")
}

// slugOfChatKey 对话的 key 是「answers:<slug>:<指纹>」「revise:<slug>:v<版本>」。slug 里不会有冒号（见 slugify）。
func slugOfChatKey(key string) string {
	parts := strings.SplitN(key, ":", 3)
	if len(parts) < 3 {
		return ""
	}
	return parts[1]
}

// snapshotProduct 收集这个产品要存的东西。调用方必须已持有 s.mu。
func (s *AppState) snapshotProduct(slug string) savedProduct {
	var out savedProduct
	for key, cs := range s.chats {
		if slugOfChatKey(key) != slug || cs.Finished || !cs.hasUser() {
			continue
		}
		if out.Chats == nil {
			out.Chats = map[string]savedChat{}
		}
		out.Chats[key] = savedChat{
			Intro: cs.Intro, Msgs: cs.Msgs, OpenChips: cs.OpenChips, Chips: cs.Chips, Replied: cs.replied,
			Card: cs.Card, Answers: cs.Answers, Title: cs.Title, Requirement: cs.Requirement,
		}
	}
	// 试玩的状态只属于当前选中的产品
	if s.selected == slug && s.playFP != "" && len(s.playMessages) > 0 && !s.playStreaming {
		out.PlayFP, out.Play, out.PlaySeq = s.playFP, s.playMessages, s.playMsgSeq
	} else if saved, ok := s.playSaved[slug]; ok {
		out.PlayFP, out.Play, out.PlaySeq = saved.PlayFP, saved.Play, saved.PlaySeq // 还没打开试玩：留着上次存的
	}
	out.Notes = s.notes[slug]
	return out
}

// saveProduct 把这个产品的对话写到磁盘；什么都没有就删掉文件。在后台 goroutine 里调。
func (s *AppState) saveProduct(slug string) {
	s.mu.Lock()
	path := storeFile(s.productPaths[slug])
	snap := s.snapshotProduct(slug)
	data, err := json.Marshal(snap)
	s.mu.Unlock()
	if path == "" || err != nil {
		return
	}
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	if snap.empty() {
		_ = os.Remove(path)
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}

// restoreProduct 第一次打开这个产品时把存下的对话读回来；内存里已有的（这次打开后新说的）不覆盖。
// 调用方必须已持有 s.mu。
func (s *AppState) restoreProduct(slug string) {
	if s.restored[slug] {
		return
	}
	s.restored[slug] = true
	path := storeFile(s.productPaths[slug])
	if path == "" {
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var saved savedProduct
	if json.Unmarshal(raw, &saved) != nil {
		return
	}
	for key, c := range saved.Chats {
		if slugOfChatKey(key) != slug {
			continue
		}
		if _, ok := s.chats[key]; ok {
			continue
		}
		cs := &chatSession{
			Intro: c.Intro, Msgs: c.Msgs, OpenChips: c.OpenChips, Chips: c.Chips, replied: c.Replied,
			Card: c.Card, Answers: c.Answers, Title: c.Title, Requirement: c.Requirement,
		}
		s.chats[key] = cs
	}
	if len(s.notes[slug]) == 0 && len(saved.Notes) > 0 {
		s.notes[slug] = saved.Notes
	}
	if saved.PlayFP != "" && len(saved.Play) > 0 {
		s.playSaved[slug] = saved
	}
}

// resumePlay 拿到试玩开场后：开场指纹和上次存的一致，就把上次的对话接上。调用方必须已持有 s.mu。
func (s *AppState) resumePlay(slug, fingerprint string) {
	saved, ok := s.playSaved[slug]
	delete(s.playSaved, slug)
	if !ok || fingerprint == "" || saved.PlayFP != fingerprint || len(s.playMessages) > 0 {
		return
	}
	s.playMessages = saved.Play
	if saved.PlaySeq > s.playMsgSeq {
		s.playMsgSeq = saved.PlaySeq
	}
	s.playChips = nil // 已经聊过了，开场的场景不再给
}
