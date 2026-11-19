import { useState, type SubmitEvent } from "react"
import { login } from "@/api/http"
import { ApiError } from "@/api/types"
import { EyeIcon, EyeOffIcon, LockClosedIcon, ServerIcon, WindowConsoleIcon } from "@/components/icons"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { BTN_PRIMARY, FIELD_INPUT, ICON_BTN } from "@/workspace/classes"

function loginMessage(e: ApiError): string {
  if (e.status === 401) return `Wrong username or password. ${e.message}`
  if (e.status === 429) {
    return e.retryAfter === null ? e.message : `Too many attempts. Try again in ${e.retryAfter} seconds. ${e.message}`
  }
  return e.message
}

// Mockup 3 screen 01: .login-stage on Mica, 420px .login-card.
export function LoginScreen({ onSignedIn }: { onSignedIn: () => void }) {
  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")
  const [showPassword, setShowPassword] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const submit = async (ev: SubmitEvent) => {
    ev.preventDefault()
    setBusy(true)
    try {
      await login(username, password)
      setError(null)
      onSignedIn()
    } catch (e) {
      if (!(e instanceof ApiError)) throw e
      setError(loginMessage(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="grid h-full place-items-center overflow-auto bg-(image:--mica) p-4">
      <Card className="block w-[420px] max-w-full gap-0 rounded-xl px-10 pt-10 pb-8 text-body shadow-28 ring-0">
        <form onSubmit={submit}>
          <span className="inline-grid size-11 place-items-center rounded-md bg-primary text-primary-foreground shadow-4">
            <WindowConsoleIcon className="size-6 shrink-0" />
          </span>
          <h1 className="mt-5 mb-1 text-title3 font-semibold">Sign in to Roosterm</h1>
          <p className="mb-7 flex items-center gap-1.5 text-muted-foreground">
            <ServerIcon className="size-4 shrink-0" /> {window.location.host} · terminal portal
          </p>
          <div className="mb-[18px] grid gap-1.5">
            <Label htmlFor="username" className="text-body leading-(--lh-body) font-semibold">Username</Label>
            <Input
              id="username"
              autoComplete="username"
              className={FIELD_INPUT}
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              required
            />
          </div>
          <div className="mb-[18px] grid gap-1.5">
            <Label htmlFor="password" className="text-body leading-(--lh-body) font-semibold">Password</Label>
            <div className="relative flex items-center">
              <Input
                id="password"
                type={showPassword ? "text" : "password"}
                autoComplete="current-password"
                className={`${FIELD_INPUT} pr-10`}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
              />
              <Button
                type="button"
                variant="ghost"
                className={`${ICON_BTN} absolute right-0.5`}
                aria-label={showPassword ? "Hide password" : "Show password"}
                aria-pressed={showPassword}
                onClick={() => setShowPassword((v) => !v)}
              >
                {showPassword ? <EyeOffIcon /> : <EyeIcon />}
              </Button>
            </div>
          </div>
          {error !== null && (
            <p role="alert" className="mb-[18px] break-all text-destructive">
              {error}
            </p>
          )}
          <Button type="submit" disabled={busy} className={`${BTN_PRIMARY} mt-2.5 h-11 w-full px-5 text-subtitle2`}>
            Sign in
          </Button>
          <div className="mt-[22px] flex gap-2 border-t border-grey-94 pt-[18px] text-caption text-muted-foreground">
            <LockClosedIcon className="size-4 shrink-0" />
            <span>Your sessions keep running on this host after you sign out. Sign-in is rate limited.</span>
          </div>
        </form>
      </Card>
    </div>
  )
}
