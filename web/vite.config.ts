import path from "node:path"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { defineConfig } from "vite"

// 构建产物直接写进 Python 包：blueprint/web/static/，由 `python -m blueprint ui` 托管。
// 开发时 `npm run dev`，接口转发给本机跑着的 `python -m blueprint ui --no-browser`。
export default defineConfig({
  base: "/static/",
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: { "@": path.resolve(__dirname, "./src") },
  },
  build: {
    outDir: "../blueprint/web/static",
    emptyOutDir: true,
    chunkSizeWarningLimit: 1600, // 本机加载，一个包够用，不拆分
  },
  server: {
    proxy: {
      // 引擎只接受本机来源的 POST：开发时页面在 5173 端口，转发前去掉 Origin，Host 改成引擎自己的
      "/api": {
        target: "http://127.0.0.1:8765",
        changeOrigin: true,
        configure: (proxy) => proxy.on("proxyReq", (req) => req.removeHeader("origin")),
      },
    },
  },
})
