"use client";

import { AuiIf, useAuiState, ThreadPrimitive } from "@assistant-ui/react";
import { useContext, type FC } from "react";

import { FallbackChipsContext } from "@/components/chat/chat-storage";

type Item = { prompt: string; title?: string | undefined; label?: string | undefined };

/** 一排快捷回复（和桌面版一样：小号描边按钮，靠左，放不下就换行）。点了就把这句原样发出去。 */
const FollowupSuggestionsRow: FC<{ suggestions: readonly Item[] }> = ({ suggestions }) => (
  <div className="aui-thread-followup-suggestions relative z-10 flex flex-wrap gap-2">
    {suggestions.map((suggestion, idx) => (
      <ThreadPrimitive.Suggestion
        key={idx}
        className="aui-thread-followup-suggestion border-border-strong hover:border-text-quaternary h-6 rounded-[6px] border bg-background px-2 text-xs whitespace-nowrap transition-colors"
        prompt={suggestion.prompt}
        send
      >
        {suggestion.title ?? suggestion.prompt}
        {suggestion.label && <span className="text-muted-foreground ms-1">{suggestion.label}</span>}
      </ThreadPrimitive.Suggestion>
    ))}
  </div>
);

/**
 * 回复下方的快捷按钮。运行时里没有建议时（开场白、从本地恢复的对话还没经过一次回复），
 * 用 ChatProvider 给的兜底按钮。
 */
export const ThreadFollowupSuggestions: FC = () => {
  const runtime = useAuiState((s) => s.thread.suggestions);
  const fallback = useContext(FallbackChipsContext);
  const items: readonly Item[] = runtime.length > 0 ? runtime : fallback.map((prompt) => ({ prompt }));
  return (
    <AuiIf condition={(s) => !s.thread.isEmpty && !s.thread.isRunning}>
      {items.length > 0 && <FollowupSuggestionsRow suggestions={items} />}
    </AuiIf>
  );
};
