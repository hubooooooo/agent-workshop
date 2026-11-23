import { SparklesIcon } from "lucide-react"

import { Thread } from "@/components/assistant-ui/elements/thread.aui"
import { ChatProvider } from "@/components/chat/chat-runtime"
import { useChatControls } from "@/components/chat/chat-storage"
import { RestartButton } from "@/components/chat/restart-button"
import { intakeToolkit } from "@/components/chat/tool-cards"
import { ModeCallouts, Screen, ScrollBody, TopBar, useSettings } from "@/components/common"
import { Button } from "@/components/ui/button"
import { chatEndpoints } from "@/lib/api"

// 欢迎语和例子和桌面版一样（desktop/view.go 的 newProductWelcome）
const STARTERS = [
  "帮社区面包店回复顾客微信的 AI 客服，能查今天的面包、接预订",
  "帮我妈记账的 AI，她发一句「买菜 35」就记下来，月底给她总结",
  "每天早上自动整理我的工作邮件，挑出要回的并起草回复",
]

function Welcome() {
  return (
    <div className="flex flex-col items-center gap-2.5 text-center">
      <SparklesIcon className="size-7 text-text-secondary" />
      <h1 className="text-2xl font-semibold">说说你想做的 AI 产品</h1>
      <p className="text-text-secondary">
        我会一次问你一个问题，问清楚后整理成需求草稿给你确认。说不清的地方直接说「不知道」就行。
      </p>
    </div>
  )
}

// 产品建好后对话就结束了；再往下聊不会改到已建的产品，所以把输入框换成「再建一个」
function Finished() {
  const { restart } = useChatControls()
  return (
    <div className="flex flex-wrap items-center justify-center gap-3 pt-3.5 pb-2 text-[13px] text-muted-foreground">
      这个产品已经开始推导。
      <Button variant="outline" size="sm" onClick={restart}>
        再新建一个产品
      </Button>
    </div>
  )
}

export function NewProductPage() {
  const { settings } = useSettings()
  if (!settings.configured) {
    return (
      <Screen>
        <TopBar title="新建产品" />
        <ScrollBody>
          <ModeCallouts />
        </ScrollBody>
      </Screen>
    )
  }
  return (
    <Screen>
      <ChatProvider endpoint={chatEndpoints.intake} toolkit={intakeToolkit} starters={STARTERS} storageKey="intake">
        <TopBar title="新建产品" sub="一次问一个问题，问清楚后整理成需求草稿" actions={<RestartButton label="重新开始" />} />
        {settings.mock && (
          <div className="mx-auto w-full max-w-[808px] px-6 pt-4">
            <ModeCallouts />
          </div>
        )}
        <div className="min-h-0 flex-1">
          <Thread components={{ Welcome, Finished }} placeholder="描述你想做的 AI 产品，比如：帮社区面包店回复顾客微信的 AI 客服" />
        </div>
      </ChatProvider>
    </Screen>
  )
}
