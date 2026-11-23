import type { ReactNode } from "react"

import { cn } from "@/lib/utils"

/** 一条条的内容：模型给的是列表，就按列表显示，不拼成一段（和桌面版的 bulletList 一样，灰色圆点）。 */
export function Bullets({ items, className }: { items: ReactNode[]; className?: string }) {
  return (
    <ul className={cn("space-y-1", className)}>
      {items.map((s, i) => (
        <li key={i} className="flex gap-2">
          <span className="shrink-0 text-muted-foreground">•</span>
          <span className="min-w-0 wrap-anywhere">{s}</span>
        </li>
      ))}
    </ul>
  )
}
