import type { ExportedMessageRepository } from "@assistant-ui/react"

import type { CardEdit } from "@/lib/api"
import { createContext, useContext } from "react"

// 新建产品的对话存在浏览器本地（还没有产品可以存）；产品的三段对话存在引擎里，见 chat-runtime.tsx。
// 卡片一确认（流程走完）就删掉，不留用完的对话。
// localStorage 在隐私模式等情况下可能不可用，所有读写都包在 try 里，失败就当没存过。

const PREFIX = "ab:chat:"

export type SavedChat = { thread: ExportedMessageRepository; chips: string[] }

function reviveDates(thread: ExportedMessageRepository): ExportedMessageRepository {
  for (const item of thread.messages) {
    const msg = item.message as { createdAt: Date | string }
    if (typeof msg.createdAt === "string") msg.createdAt = new Date(msg.createdAt)
  }
  return thread
}

export function loadChat(key: string): SavedChat | null {
  try {
    const raw = localStorage.getItem(PREFIX + key)
    if (!raw) return null
    const data = JSON.parse(raw) as SavedChat
    return data.thread?.messages?.length ? { thread: reviveDates(data.thread), chips: data.chips ?? [] } : null
  } catch {
    return null
  }
}

export function saveChat(key: string, data: SavedChat) {
  try {
    localStorage.setItem(PREFIX + key, JSON.stringify(data))
  } catch {
    /* 存不了就算了，只是少了「接着聊」 */
  }
}

export function clearChat(key: string) {
  try {
    localStorage.removeItem(PREFIX + key)
  } catch {
    /* 同上 */
  }
}

/** 一组字符串的短指纹，用来区分「哪一版方案的问题」。 */
export function fingerprint(items: readonly string[]) {
  let h = 5381
  for (const ch of items.join("\u0000")) h = ((h << 5) + h + ch.charCodeAt(0)) | 0
  return (h >>> 0).toString(36)
}

/** 回复下方快捷按钮的兜底：开场白、或从本地恢复的对话，还没经过一次回复，运行时里没有建议。 */
export const FallbackChipsContext = createContext<readonly string[]>([])

export const ChatControlsContext = createContext<{ restart: () => void } | null>(null)

export function useChatControls() {
  const ctx = useContext(ChatControlsContext)
  if (!ctx) throw new Error("ChatControlsContext missing")
  return ctx
}

/** 卡片上改过、还没确认的内容：随对话一起存进引擎，换到桌面版打开也还在。 */
export const CardEditContext = createContext<{ initial?: CardEdit; set: (edit: CardEdit) => void }>({ set: () => {} })

export function useCardEdit() {
  return useContext(CardEditContext)
}
