// Reconnect triggers per spec: network back online, or page back in the foreground. No timers.
export function onResume(callback: () => void): () => void {
  const onVisibility = () => {
    if (document.visibilityState === "visible") callback()
  }
  window.addEventListener("online", callback)
  document.addEventListener("visibilitychange", onVisibility)
  return () => {
    window.removeEventListener("online", callback)
    document.removeEventListener("visibilitychange", onVisibility)
  }
}
