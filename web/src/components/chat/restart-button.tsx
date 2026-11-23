import { useAuiState } from "@assistant-ui/react"
import { useState } from "react"

import { useChatControls } from "@/components/chat/chat-storage"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"

/** 「重新开始」：对话里有用户发过的话、而且没在回复时才显示；点了先确认（和桌面版一样的确认框）。 */
export function RestartButton({ label = "重新开始" }: { label?: string }) {
  const { restart } = useChatControls()
  const [asking, setAsking] = useState(false)
  const visible = useAuiState((s) => !s.thread.isRunning && s.thread.messages.some((m) => m.role === "user"))
  if (!visible) return null
  return (
    <>
      <Button variant="ghost" size="sm" onClick={() => setAsking(true)}>
        {label}
      </Button>
      <Dialog open={asking} onOpenChange={setAsking}>
        <DialogContent className="sm:max-w-[400px]">
          <DialogHeader>
            <DialogTitle>重新开始</DialogTitle>
          </DialogHeader>
          <p>清空这段对话，从头开始？</p>
          <DialogFooter>
            <Button variant="outline" size="sm" className="h-8 px-3" onClick={() => setAsking(false)}>取消</Button>
            <Button
              size="sm"
              className="h-8 px-3"
              onClick={() => {
                setAsking(false)
                restart()
              }}
            >
              确定
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
