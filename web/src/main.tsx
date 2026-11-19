import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import "@xterm/xterm/css/xterm.css"
import "./styles/globals.css"
import App from "./App"

const root = document.getElementById("root")
if (!root) throw new Error("main: expected element #root in index.html, found none")
createRoot(root).render(<StrictMode><App /></StrictMode>)
