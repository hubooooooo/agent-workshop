import {
  AssistantRuntimeProvider,
  AuiConfig,
  Suggestions,
  Tools,
  useLocalRuntime,
  type ChatModelAdapter,
  type DataMessagePart,
  type SuggestionAdapter,
  type TextMessagePart,
  type ThreadMessage,
  type ThreadMessageLike,
  type Toolkit,
  type ToolCallMessagePart,
} from "@assistant-ui/react"
import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react"

import {
  CardEditContext,
  ChatControlsContext,
  clearChat,
  FallbackChipsContext,
  loadChat,
  saveChat,
} from "@/components/chat/chat-storage"
import { api, streamChat, type CardEdit, type ConversationKind, type WireMessage, type WireSession } from "@/lib/api"

// -- 引擎里的存档格式 ↔ assistant-ui 的消息 ----------------------------------------------------

/** 存档里的消息换成 assistant-ui 的消息。最后一条回复里还没确认的卡片，要让运行时知道它在等用户操作。 */
function fromWire(session: WireSession): ThreadMessageLike[] {
  const last = session.messages.length - 1
  return session.messages.map((m, i) => {
    const content = m.parts.flatMap((p): Exclude<ThreadMessageLike["content"], string>[number][] => {
      if (p.type === "text") return [{ type: "text", text: p.text }]
      if (p.type === "data") return [{ type: "data", name: p.name, data: p.data ?? {} }]
      if (p.type === "tool") {
        const args = (p.args ?? {}) as ToolCallMessagePart["args"]
        return [{
          type: "tool-call",
          toolCallId: `${m.id}-${p.name}`,
          toolName: p.name,
          args,
          argsText: JSON.stringify(args),
          ...(p.result !== undefined ? { result: p.result } : {}),
        }]
      }
      return []
    })
    const pending = i === last && m.parts.some((p) => p.type === "tool" && p.result === undefined)
    return {
      id: m.id,
      role: m.role,
      content,
      ...(pending ? { status: { type: "requires-action" as const, reason: "tool-calls" as const } } : {}),
    }
  })
}

function toWireMessages(messages: readonly ThreadMessage[]): WireMessage[] {
  return messages
    .filter((m) => m.role === "user" || m.role === "assistant")
    .map((m) => ({
      id: m.id,
      role: m.role as "user" | "assistant",
      parts: m.content.flatMap((p): WireMessage["parts"] => {
        if (p.type === "text") return [{ type: "text", text: p.text }]
        if (p.type === "data") return [{ type: "data", name: p.name, data: p.data as Record<string, unknown> }]
        if (p.type === "tool-call") {
          return [{ type: "tool", name: p.toolName, args: p.args as Record<string, unknown>, ...(p.result !== undefined ? { result: p.result } : {}) }]
        }
        return []
      }),
    }))
}

/**
 * 只把文字发给服务端；卡片（tool-call）是网页自己的事，服务端不需要。
 * 带 wire 字段的数据卡片（试玩里模拟的动作）按原顺序换成文字一起发回，下一轮模型才知道之前发生了什么。
 */
function toWire(messages: readonly ThreadMessage[]) {
  return messages.map((m) => ({
    role: m.role,
    text: m.content
      .flatMap((p) => {
        if (p.type === "text") return [p.text]
        if (p.type === "data" && typeof p.data?.wire === "string") return [p.data.wire]
        return []
      })
      .join("\n"),
  }))
}

type ChatProviderProps = {
  endpoint: string
  toolkit: Toolkit
  /** 存在引擎里（产品的三段对话，网页版和桌面版共用）：哪个产品、哪段对话。 */
  persist?: { slug: string; kind: ConversationKind }
  /** 只存在这个浏览器里用的键（新建产品的对话，还没有产品可以存）。 */
  storageKey?: string
  initialMessages?: ThreadMessageLike[]
  /** 开场就显示的快捷按钮（开场白由网页直接放入，还没经过一次回复时用）。 */
  initialChips?: string[]
  starters?: string[]
  children: ReactNode
}

/**
 * 把一段对话接到服务端的 NDJSON 接口上（blueprint/chat.py）。
 *
 * - 文字逐段显示；服务端发来的快捷按钮在回复结束后出现在输入框上方。
 * - 服务端要求确认（tool 事件）时弹出卡片并暂停对话；用户在卡片上点了确认，对话就结束，不再请求服务端。
 * - 给了 persist 就存在引擎里（网页版和桌面版共用），给了 storageKey 就存在浏览器本地；
 *   切走再回来、刷新页面都能接着聊；「重新开始」清空重来。
 */
export function ChatProvider(props: ChatProviderProps) {
  const [generation, setGeneration] = useState(0)
  // 存在引擎里的对话：先读回来再建运行时（undefined = 还在读，null = 没有存档）
  const [loaded, setLoaded] = useState<WireSession | null | undefined>(props.persist ? undefined : null)
  const persistKey = props.persist ? `${props.persist.slug}:${props.persist.kind}` : ""
  useEffect(() => {
    if (!props.persist) return
    const { slug, kind } = props.persist
    let live = true
    setLoaded(undefined)
    api.conversations(slug).then(
      (all) => live && setLoaded(all[kind]),
      () => live && setLoaded(null), // 读不到就从头开始
    )
    return () => {
      live = false
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [persistKey])

  const controls = useMemo(
    () => ({
      restart: () => {
        if (props.storageKey) clearChat(props.storageKey)
        if (props.persist) {
          api.saveConversation(props.persist.slug, props.persist.kind, null).catch(() => {})
          setLoaded(null)
        }
        setGeneration((g) => g + 1)
      },
    }),
    [props.storageKey, props.persist],
  )
  if (loaded === undefined) return <p className="py-10 text-center text-[13px] text-muted-foreground">读取中…</p>
  return (
    <ChatControlsContext.Provider value={controls}>
      <ChatSession key={generation} {...props} loaded={loaded} />
    </ChatControlsContext.Provider>
  )
}

function ChatSession({
  endpoint,
  toolkit,
  persist,
  storageKey,
  initialMessages,
  initialChips,
  starters,
  children,
  loaded,
}: ChatProviderProps & { loaded: WireSession | null }) {
  const [saved] = useState(() => (storageKey ? loadChat(storageKey) : null))
  const restoredChips = loaded ? (loaded.chips ?? []) : saved?.chips
  const followUps = useRef<string[]>(restoredChips ?? [])
  // 恢复的对话或开场白还没经过一次回复，运行时里没有建议，先用这组；一开始新的回复就清掉
  const [fallbackChips, setFallbackChips] = useState<readonly string[]>(restoredChips ?? initialChips ?? [])
  // 开场白不存：它每次由网页按方案生成，存档里只有用户来回说的部分
  const introCount = initialMessages?.length ?? 0
  const [messages] = useState(() => (loaded ? [...(initialMessages ?? []), ...fromWire(loaded)] : initialMessages))
  const cardEdit = useRef<CardEdit | undefined>(loaded?.card)
  const lastSaved = useRef(JSON.stringify(loaded ?? null))

  const adapter = useMemo<ChatModelAdapter>(
    () => ({
      async *run({ messages, abortSignal, unstable_getMessage }) {
        const answered = unstable_getMessage().content.some(
          (p) => p.type === "tool-call" && p.result !== undefined,
        )
        if (answered) return

        setFallbackChips([])
        followUps.current = []
        // 文字、卡片按服务端发来的顺序排；连续的文字并成一段
        const parts: (ToolCallMessagePart | DataMessagePart | TextMessagePart)[] = []
        let tools = 0
        const content = () => parts.map((p) => ({ ...p }))

        for await (const event of streamChat(endpoint, toWire(messages), abortSignal)) {
          if (event.type === "text") {
            const last = parts.at(-1)
            if (last?.type === "text") parts[parts.length - 1] = { ...last, text: last.text + event.delta }
            else parts.push({ type: "text", text: event.delta })
            yield { content: content() }
          } else if (event.type === "tool") {
            tools += 1
            parts.push({
              type: "tool-call",
              toolCallId: `${event.name}-${tools}-${Date.now()}`,
              toolName: event.name,
              args: event.args as ToolCallMessagePart["args"],
              argsText: JSON.stringify(event.args),
            })
            yield { content: content() }
          } else if (event.type === "data") {
            parts.push({ type: "data", name: event.name, data: event.data })
            yield { content: content() }
          } else if (event.type === "suggestions") {
            followUps.current = event.items
          } else if (event.type === "error") {
            throw new Error(event.message)
          }
        }
        if (tools) {
          yield { content: content(), status: { type: "requires-action", reason: "tool-calls" } }
        }
      },
    }),
    [endpoint],
  )

  const suggestion = useMemo<SuggestionAdapter>(
    () => ({ generate: async () => followUps.current.map((prompt) => ({ prompt })) }),
    [],
  )

  const runtime = useLocalRuntime(adapter, {
    initialMessages: messages,
    adapters: { suggestion },
    unstable_humanToolNames: Object.keys(toolkit),
  })

  // 存到引擎：没在回复时，对话有变化就存一份；流程走完（卡片确认了）或者还没说过话就删掉
  const persistNow = useCallback(() => {
    if (!persist) return
    const state = runtime.thread.getState()
    if (state.isRunning) return
    const done = state.messages.some((m) => m.content.some((p) => p.type === "tool-call" && p.result !== undefined))
    const own = state.messages.slice(introCount)
    let session: WireSession | null = null
    if (!done && own.some((m) => m.role === "user")) {
      const pendingCard = own.at(-1)?.content.some((p) => p.type === "tool-call" && p.result === undefined)
      session = {
        messages: toWireMessages(own),
        ...(pendingCard ? (cardEdit.current ? { card: cardEdit.current } : {}) : { chips: followUps.current }),
      }
    }
    const json = JSON.stringify(session)
    if (json === lastSaved.current) return
    lastSaved.current = json
    api.saveConversation(persist.slug, persist.kind, session).catch(() => {
      lastSaved.current = "" // 存不上下次再试
    })
  }, [persist, runtime, introCount])

  useEffect(() => {
    if (persist) return runtime.thread.subscribe(persistNow)
    if (!storageKey) return
    if (saved) runtime.thread.import(saved.thread)
    return runtime.thread.subscribe(() => {
      const state = runtime.thread.getState()
      if (state.isRunning) return
      const done = state.messages.some((m) =>
        m.content.some((p) => p.type === "tool-call" && p.result !== undefined),
      )
      if (done) clearChat(storageKey)
      else if (state.messages.some((m) => m.role === "user")) {
        saveChat(storageKey, { thread: runtime.thread.export(), chips: followUps.current })
      }
    })
  }, [runtime, storageKey, saved, persist, persistNow])

  const cardEditApi = useMemo(
    () => ({
      initial: loaded?.card,
      set: (edit: CardEdit) => {
        cardEdit.current = edit
        persistNow()
      },
    }),
    [loaded, persistNow],
  )

  const config = useMemo(
    () =>
      AuiConfig({
        tools: Tools({ toolkit }),
        ...(starters ? { suggestions: Suggestions(starters) } : {}),
      }),
    [toolkit, starters],
  )

  return (
    <AssistantRuntimeProvider runtime={runtime} config={config}>
      <CardEditContext.Provider value={cardEditApi}>
        <FallbackChipsContext.Provider value={fallbackChips}>{children}</FallbackChipsContext.Provider>
      </CardEditContext.Provider>
    </AssistantRuntimeProvider>
  )
}
