import { ChevronDownIcon } from "lucide-react"
import { useState, type ReactNode } from "react"

import { Callout, Screen, TopBar, useSettings } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { api } from "@/lib/api"
import { cn } from "@/lib/utils"

// 和桌面版的模型设置页一致：标题和说明，一张表单卡片（服务商、模型名、API Key、接口地址），下面是结果和保存按钮。

/** 表单里的一项：标签、控件、一句灰色说明。 */
function FormField({ label, htmlFor, help, children }: { label: string; htmlFor: string; help?: ReactNode; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-1.5">
      <label htmlFor={htmlFor} className="text-[13px] font-medium">{label}</label>
      {children}
      {help && <div className="text-xs text-muted-foreground">{help}</div>}
    </div>
  )
}

export function SettingsPage() {
  const { settings, reload } = useSettings()
  const [provider, setProvider] = useState(settings.provider || "deepseek")
  const providerOf = (key: string) => settings.providers.find((p) => p.key === key)!
  // 模型名留空就用服务商推荐的默认模型
  const [model, setModel] = useState(settings.provider && settings.model !== providerOf(settings.provider).model ? settings.model : "")
  const [key, setKey] = useState("")
  const [baseUrl, setBaseUrl] = useState(settings.base_url || "")
  const [busy, setBusy] = useState(false)
  const [status, setStatus] = useState("")

  const save = async () => {
    setBusy(true)
    setStatus("")
    try {
      const res = await api.saveSettings({ provider, api_key: key, model, base_url: baseUrl })
      setStatus(res.ok ? `已保存：${res.message}` : res.message)
      if (res.ok) {
        setKey("")
        await reload()
      }
    } catch (err) {
      setStatus(`保存失败：${(err as Error).message}`)
    }
    setBusy(false)
  }

  const current = providerOf(provider)
  // 已保存的 Key 只在接口格式相同时沿用：换成另一种格式要重新粘贴
  const saved = settings.provider ? providerOf(settings.provider) : undefined
  const keyReusable = Boolean(settings.key_masked) && saved?.format === current.format

  return (
    <Screen>
      <TopBar title="模型设置" />
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto flex w-full max-w-[608px] flex-col gap-4 px-6 pt-7 pb-10">
          <div className="flex flex-col gap-1.5">
            <h1 className="text-[22px] font-semibold">选择模型</h1>
            <p className="text-text-secondary">
              新建产品时的追问、生成方案和试玩，都用这里配置的模型。一份方案约 6–11 次调用、3–7 万 tokens；每次推导最多 40 次调用、150 万 tokens，超了会自动停下，不会无限扣费。
            </p>
          </div>
          {settings.mock && (
            <Callout tone="warn" className="mb-0" title="离线演示模式">
              当前不调用模型。这里保存的配置会写进本机 .env，下次正常启动时生效。
            </Callout>
          )}

          <div className="flex flex-col gap-[18px] rounded-xl border p-5">
            <FormField
              label="模型服务商"
              htmlFor="provider"
              help={
                current.key === "openai"
                  ? "OpenAI 官方接口在国内一般连不上；用中转的，选「OpenAI 格式的中转或其他服务」。"
                  : "选一个你有 API Key 的服务商。"
              }
            >
              <div className="relative">
                <select
                  id="provider"
                  value={provider}
                  onChange={(e) => {
                    setProvider(e.target.value)
                    setModel("")
                  }}
                  className="h-9 w-full appearance-none rounded-lg border border-input bg-background ps-3 pe-9 outline-none hover:border-[#b0b0b0] focus-visible:border-[#b0b0b0]"
                >
                  {settings.providers.map((p) => (
                    <option key={p.key} value={p.key}>{p.name}</option>
                  ))}
                </select>
                <ChevronDownIcon className="pointer-events-none absolute end-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              </div>
            </FormField>

            <FormField label="模型名" htmlFor="model" help="不填就用服务商推荐的默认模型。">
              <Input
                id="model"
                value={model}
                onChange={(e) => setModel(e.target.value)}
                placeholder={`留空用默认模型（${current.model}）`}
                className="h-9"
                spellCheck={false}
              />
            </FormField>

            <FormField
              label="API Key"
              htmlFor="key"
              help={
                <>
                  已保存的 Key 不会显示出来，重新粘贴才会覆盖。Key 只保存在这台电脑的 .env 文件里。
                  {current.key_page ? (
                    <a className="mt-1.5 block w-fit text-link hover:underline" href={current.key_page} target="_blank" rel="noopener">
                      去申请 Key →
                    </a>
                  ) : (
                    " Key 和接口地址向你用的中转服务商要。"
                  )}
                </>
              }
            >
              <Input
                id="key"
                type="password"
                autoComplete="off"
                value={key}
                onChange={(e) => setKey(e.target.value)}
                placeholder={keyReusable ? `已保存（${settings.key_masked}），不改就留空` : "粘贴 API Key"}
                className="h-9"
              />
            </FormField>

            {current.custom_base && (
              <FormField
                label="接口地址"
                htmlFor="base"
                help="填到 /v1 为止，引擎会自己接上 /chat/completions。DeepSeek、Kimi 的 OpenAI 格式地址也可以填在这里。"
              >
                <Input id="base" value={baseUrl} onChange={(e) => setBaseUrl(e.target.value)} placeholder="https://…/v1" className="h-9" spellCheck={false} />
              </FormField>
            )}
          </div>

          <div className="flex items-center gap-3">
            <span
              className={cn(
                "min-w-0 flex-1 text-[13px] text-text-secondary",
                status.startsWith("已保存") && "text-[#00864e]",
                status.startsWith("保存失败") && "text-bad",
              )}
            >
              {busy ? "正在测试连接…" : status}
            </span>
            <Button onClick={save} disabled={busy}>
              测试连接并保存
            </Button>
          </div>
        </div>
      </div>
    </Screen>
  )
}
