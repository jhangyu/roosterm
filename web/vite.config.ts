import path from "path"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { defineConfig } from "vite"

// Must equal the Go host's `//go:embed all:dist` directory (host/internal/web/static.go).
const EMBED_DIR = path.resolve(import.meta.dirname, "../host/internal/web/dist")
// Default `listen` of `roosterm serve` (backend config); the dev server proxies to it.
const HOST_DEV_URL = "http://127.0.0.1:8787"

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": path.resolve(import.meta.dirname, "./src") } },
  build: { outDir: EMBED_DIR, emptyOutDir: true },
  server: {
    proxy: {
      "/api": HOST_DEV_URL,
      "/login": HOST_DEV_URL,
      "/logout": HOST_DEV_URL,
      "/ws": { target: HOST_DEV_URL, ws: true },
    },
  },
})
