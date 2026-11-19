import { useCallback, useEffect, useRef, useState, type KeyboardEvent, type ReactNode } from "react"
import { DropdownMenu } from "radix-ui"
import { listDirs, type DirEntry } from "@/api/http"
import { ApiError } from "@/api/types"
import {
  ArrowClockwiseIcon,
  ArrowUpIcon,
  ChevronRightIcon,
  DismissIcon,
  ErrorCircleIcon,
  EyeIcon,
  EyeOffIcon,
  FolderIcon,
  FolderOpenIcon,
  HistoryIcon,
  LockClosedIcon,
  MoreHorizontalIcon,
  SearchIcon,
  WarningIcon,
} from "@/components/icons"
import { Button } from "@/components/ui/button"
import { BTN } from "./classes"
import {
  baseName,
  childPath,
  displayPath,
  loadMemory,
  parentPath,
  saveMemory,
  segments,
  withoutRecent,
  withRecent,
  type FolderMemory,
} from "./folders"

// Mockup 4b (docs/mockups/4b-folder-drill.html): drill-down folder navigator. The folder the breadcrumb ends in is
// the session's start folder.

export type Listing =
  | { kind: "loading" }
  | { kind: "ready"; entries: DirEntry[] }
  | { kind: "denied"; message: string } // 403 permission_denied: still a valid start folder
  | { kind: "error"; message: string }
  | { kind: "gone" } // 404: the only state that blocks Open

type Drill = "in" | "up" | "none"

// Case-insensitive natural sort (v2 before v10).
const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: "base" })

export type FolderBrowser = {
  path: string
  listing: Listing
  drill: Drill
  // The remembered folder that no longer exists when the dialog opened (its nearest parent is shown instead).
  notice: string | null
  memory: FolderMemory
  filter: string
  setFilter: (filter: string) => void
  go: (path: string) => void
  up: () => void
  retry: () => void
  toggleHidden: () => void
  // Store the current folder (and the hidden-folder toggle) as this device's last used folder.
  remember: () => void
}

export function useFolderBrowser(open: boolean, onUnauthorized: () => void): FolderBrowser {
  const [path, setPath] = useState("")
  const [listing, setListing] = useState<Listing>({ kind: "loading" })
  const [drill, setDrill] = useState<Drill>("none")
  const [notice, setNotice] = useState<string | null>(null)
  const [memory, setMemory] = useState<FolderMemory>({ recent: [], showHidden: false })
  const [filter, setFilter] = useState("")
  // Only the latest listing request may update the picker.
  const seq = useRef(0)

  // Lists `target`; with walkUp a missing folder is replaced by its nearest existing ancestor.
  // Resolves to the folder finally shown, or null when superseded by a newer request.
  const load = useCallback(
    async (target: string, nextDrill: Drill, walkUp: boolean): Promise<string | null> => {
      const id = ++seq.current
      setPath(target)
      setDrill(nextDrill)
      setFilter("")
      setListing({ kind: "loading" })
      let p = target
      for (;;) {
        try {
          const entries = await listDirs(p)
          if (id !== seq.current) return null
          setPath(p)
          setListing({ kind: "ready", entries: [...entries].sort((a, b) => collator.compare(a.name, b.name)) })
          return p
        } catch (e) {
          if (!(e instanceof ApiError)) throw e
          if (id !== seq.current) return null
          if (e.status === 401) {
            onUnauthorized()
            return null
          }
          if (e.status === 404 && walkUp && p !== "") {
            p = parentPath(p)
            continue
          }
          setPath(p)
          if (e.status === 404) setListing({ kind: "gone" })
          else if (e.status === 403 && e.kind === "permission_denied") setListing({ kind: "denied", message: e.detail })
          else setListing({ kind: "error", message: e.message })
          return p
        }
      }
    },
    [onUnauthorized],
  )

  useEffect(() => {
    if (!open) return
    const remembered = loadMemory()
    const start = remembered.recent[0] ?? ""
    setMemory(remembered)
    setNotice(null)
    void load(start, "none", true).then((shown) => {
      if (shown === null || shown === start) return
      setNotice(start)
      // The dead entry leaves Recent; storage is rewritten on the next Open.
      setMemory((m) => ({ ...m, recent: withoutRecent(m.recent, start) }))
    })
    return () => {
      seq.current++
    }
  }, [open, load])

  const go = (next: string) => {
    setNotice(null)
    void load(next, segments(next).length >= segments(path).length ? "in" : "up", false)
  }
  return {
    path,
    listing,
    drill,
    notice,
    memory,
    filter,
    setFilter,
    go,
    up: () => {
      setNotice(null)
      if (path !== "") void load(parentPath(path), "up", true)
    },
    retry: () => void load(path, "none", false),
    toggleHidden: () => setMemory((m) => ({ ...m, showHidden: !m.showHidden })),
    remember: () => saveMemory({ recent: withRecent(memory.recent, path), showHidden: memory.showHidden }),
  }
}

// Focus ring shared by rows, crumbs and chips (mockup 4b: 2px grey-14 ring + white inset).
const FOCUS = "outline-none focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-foreground focus-visible:shadow-[inset_0_0_0_3px_var(--card)]"
// Colour transitions 150 ms (Fluent easy-ease); no double-tap zoom delay on touch.
const TRANSITION = "touch-manipulation transition-colors duration-150 ease-[cubic-bezier(0.33,0,0.67,1)]"
const RECOVERY_BTN = `${BTN} max-md:h-11`
// Overflow-menu indent: 10 px base, 12 px per depth.
const MENU_PAD_PX = 10
const MENU_INDENT_PX = 12
// Phone: the menu spans the picker bar (20 px from each screen edge).
const MENU_COLLISION_PX = 20
const HIDDEN_NAMES_SHOWN = 3
export const KBD = "rounded-xs border border-b-2 border-grey-88 bg-card px-[5px] font-mono text-[11px] text-grey-38"

const plural = (n: number, one: string) => `${n} ${one}${n === 1 ? "" : "s"}`

// Recent chips: jump anywhere; the highlighted chip is the folder you are in. A shared folder name gets its parent's name.
export function RecentFolders({ b }: { b: FolderBrowser }) {
  const recent = b.memory.recent
  if (recent.length === 0) return null
  return (
    <div className="mb-2.5 flex shrink-0 items-center gap-2 overflow-x-auto [scrollbar-width:none]" aria-label="Recent folders">
      <span className="inline-flex shrink-0 items-center gap-1 text-caption text-muted-foreground">
        <HistoryIcon className="size-4" />
        Recent
      </span>
      {recent.map((p) => {
        const name = baseName(p)
        const shared = recent.some((q) => q !== p && baseName(q) === name)
        const on = p === b.path
        return (
          <button
            key={p}
            type="button"
            title={displayPath(p)}
            aria-current={on ? "location" : undefined}
            onClick={() => b.go(p)}
            className={`inline-flex h-8 shrink-0 items-center gap-1.5 rounded-full border px-3 text-[13px] font-semibold whitespace-nowrap max-md:h-9 ${FOCUS} ${TRANSITION} ${
              on ? "border-brand-150 bg-brand-160 text-brand-60" : "border-grey-88 bg-card text-grey-26 hover:bg-grey-96 active:bg-grey-92"
            }`}
          >
            {name}
            {shared && <small className="font-normal text-muted-foreground">{baseName(parentPath(p))}</small>}
          </button>
        )
      })}
    </div>
  )
}

const CRUMB = `h-(--picker-crumb-h) shrink-0 rounded-sm px-2 leading-(--picker-crumb-h) whitespace-nowrap max-md:h-(--picker-crumb-h-touch) max-md:px-2.5 max-md:leading-(--picker-crumb-h-touch) ${FOCUS} ${TRANSITION}`
const CRUMB_LINK = `${CRUMB} max-w-(--picker-crumb-max) truncate text-grey-38 hover:bg-foreground/5 hover:text-foreground active:bg-foreground/8 max-md:max-w-(--picker-crumb-max-touch)`

function Sep({ className = "inline-grid" }: { className?: string }) {
  return (
    <span className={`${className} shrink-0 text-grey-74`} aria-hidden="true">
      <ChevronRightIcon className="size-3" />
    </span>
  )
}

// Always ~, the parent and the current folder; the ancestors between collapse into "…" past 3 levels
// (phone: ~, "…" and the current folder past 2 levels).
// ponytail: collapse by depth, not by measured overflow; long names are capped by --picker-crumb-max instead.
function Crumbs({ b }: { b: FolderBrowser }) {
  const segs = segments(b.path)
  const current = segs.length - 1
  const parent = segs.length - 2
  const deskCollapse = segs.length > 3
  const phoneCollapse = segs.length > 2
  const inDeskMenu = (i: number) => deskCollapse && i < parent
  const inPhoneMenu = (i: number) => phoneCollapse && i < current
  const shown = (i: number) => `${inPhoneMenu(i) ? "hidden" : "flex"} ${inDeskMenu(i) ? "md:hidden" : "md:flex"}`
  const target = (i: number) => segs.slice(0, i + 1).join("/")
  const collapsed = segs.slice(0, current).length
  const menuVisible = `${phoneCollapse ? "inline-grid" : "hidden"} ${deskCollapse ? "md:inline-grid" : "md:hidden"}`

  return (
    <nav className="flex min-w-0 flex-1 items-center" aria-label="Folder path">
      {segs.length === 0 ? (
        <CurrentCrumb name="~" home />
      ) : (
        <button type="button" className={`${CRUMB_LINK} font-mono font-semibold`} aria-label="Home folder" onClick={() => b.go("")}>
          ~
        </button>
      )}
      {(deskCollapse || phoneCollapse) && (
        <>
          <Sep className={menuVisible} />
          <DropdownMenu.Root>
            <DropdownMenu.Trigger
              aria-label={`${collapsed} more folders`}
              className={`${menuVisible} ${CRUMB} w-9 place-items-center px-0 text-grey-38 hover:bg-foreground/5 data-[state=open]:bg-foreground/8 data-[state=open]:text-foreground max-md:w-11 max-md:px-0`}
            >
              <MoreHorizontalIcon />
            </DropdownMenu.Trigger>
            <DropdownMenu.Portal>
              <DropdownMenu.Content
                align="start"
                sideOffset={4}
                collisionPadding={MENU_COLLISION_PX}
                aria-label="Collapsed folders"
                className="z-50 min-w-[220px] rounded-xs bg-popover p-1 text-body text-foreground shadow-16 max-md:w-[calc(100vw-40px)]"
              >
                {segs.slice(0, current).map((name, i) => (
                  <DropdownMenu.Item
                    key={target(i)}
                    onSelect={() => b.go(target(i))}
                    style={{ paddingLeft: MENU_PAD_PX + MENU_INDENT_PX * i }}
                    className={`${inPhoneMenu(i) ? "flex" : "hidden"} ${inDeskMenu(i) ? "md:flex" : "md:hidden"} h-9 cursor-pointer items-center gap-2.5 rounded-xs pr-2.5 outline-none data-highlighted:bg-grey-96 max-md:h-12`}
                  >
                    <FolderIcon className="size-4 shrink-0 text-grey-38" />
                    <span className="truncate">{name}</span>
                  </DropdownMenu.Item>
                ))}
              </DropdownMenu.Content>
            </DropdownMenu.Portal>
          </DropdownMenu.Root>
        </>
      )}
      {segs.slice(0, current).map((name, i) => (
        <span key={target(i)} className={`${shown(i)} min-w-0 shrink-0 items-center`}>
          <Sep />
          <button type="button" className={CRUMB_LINK} title={name} onClick={() => b.go(target(i))}>
            {name}
          </button>
        </span>
      ))}
      {segs.length > 0 && (
        <>
          <Sep />
          <CurrentCrumb name={segs[current]} />
        </>
      )}
    </nav>
  )
}

function CurrentCrumb({ name, home = false }: { name: string; home?: boolean }) {
  return (
    <span
      aria-current="location"
      aria-label={home ? "Home folder" : undefined}
      title={name}
      className={`relative block h-(--picker-crumb-h) min-w-0 truncate px-2 leading-(--picker-crumb-h) font-semibold text-foreground after:absolute after:inset-x-2 after:bottom-0.5 after:h-[3px] after:rounded-full after:bg-primary max-md:h-(--picker-crumb-h-touch) max-md:px-2.5 max-md:leading-(--picker-crumb-h-touch) ${home ? "font-mono" : ""}`}
    >
      {name}
    </span>
  )
}

function Highlight({ name, query }: { name: string; query: string }) {
  const at = query === "" ? -1 : name.toLowerCase().indexOf(query)
  if (at < 0) return <>{name}</>
  return (
    <>
      {name.slice(0, at)}
      <mark className="rounded-xs bg-brand-150 px-px text-inherit">{name.slice(at, at + query.length)}</mark>
      {name.slice(at + query.length)}
    </>
  )
}

function Panel(props: { icon: ReactNode; tone?: "plain" | "error"; alert?: boolean; title: string; children?: ReactNode; actions?: ReactNode }) {
  return (
    <div
      role={props.alert ? "alert" : "status"}
      className="grid min-h-full content-center justify-items-center gap-1.5 px-6 py-4 text-center text-muted-foreground"
    >
      <span
        className={`mb-1 grid size-12 place-items-center rounded-md ${props.tone === "error" ? "bg-destructive-bg text-destructive" : "bg-grey-96 text-grey-38"}`}
      >
        {props.icon}
      </span>
      <b className="text-subtitle2 font-semibold text-foreground">{props.title}</b>
      {props.children}
      {props.actions !== undefined && <div className="mt-2 flex gap-2">{props.actions}</div>}
    </div>
  )
}

function UpButton({ b, label }: { b: FolderBrowser; label: string }) {
  if (b.path === "") return null
  return (
    <Button variant="outline" className={RECOVERY_BTN} onClick={b.up}>
      <ArrowUpIcon className="size-4" />
      {label}
    </Button>
  )
}

function RetryButton({ b }: { b: FolderBrowser }) {
  return (
    <Button variant="ghost" className={`${RECOVERY_BTN} border-transparent bg-transparent hover:bg-foreground/5`} onClick={b.retry}>
      <ArrowClockwiseIcon className="size-4" />
      Retry
    </Button>
  )
}

export function FolderPicker({ b, className = "" }: { b: FolderBrowser; className?: string }) {
  const filterRef = useRef<HTMLInputElement>(null)
  const listRef = useRef<HTMLDivElement>(null)

  const here = baseName(b.path)
  const parentName = baseName(parentPath(b.path))
  const entries = b.listing.kind === "ready" ? b.listing.entries : []
  const hiddenEntries = entries.filter((e) => e.hidden)
  // Dot folders show when asked for, when the filter starts with ".", or when already inside a dot folder.
  const autoHidden = b.filter.startsWith(".") || segments(b.path).some((s) => s.startsWith("."))
  const showHidden = b.memory.showHidden || autoHidden
  const pool = showHidden ? entries : entries.filter((e) => !e.hidden)
  const query = b.filter.toLowerCase()
  const prefix = pool.filter((e) => e.name.toLowerCase().startsWith(query))
  const inner = pool.filter((e) => !e.name.toLowerCase().startsWith(query) && e.name.toLowerCase().includes(query))
  const matches = query === "" ? pool : [...prefix, ...inner]
  const hiddenMatches = query === "" || showHidden ? 0 : hiddenEntries.filter((e) => e.name.toLowerCase().includes(query)).length
  const ready = b.listing.kind === "ready"

  const goAndFocus = (next: string) => {
    b.go(next)
    listRef.current?.scrollTo({ top: 0 })
    listRef.current?.focus({ preventScroll: true })
  }
  const upAndFocus = () => {
    b.up()
    listRef.current?.focus({ preventScroll: true })
  }

  // Desktop keyboard drive: typing focuses the filter; Up/Down move; Enter or Right enter; Backspace or Left go up.
  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.ctrlKey || e.metaKey || e.altKey || e.nativeEvent.isComposing) return
    const t = e.target as HTMLElement
    const inFilter = t === filterRef.current
    const rows = Array.from(listRef.current?.querySelectorAll<HTMLButtonElement>("[data-folder-row]") ?? [])
    const i = rows.indexOf(t as HTMLButtonElement)
    switch (e.key) {
      case "ArrowDown":
        if (inFilter || i >= 0 || t === listRef.current) {
          e.preventDefault()
          rows[Math.min(i + 1, rows.length - 1)]?.focus()
        }
        return
      case "ArrowUp":
        if (i >= 0) {
          e.preventDefault()
          if (i === 0) filterRef.current?.focus()
          else rows[i - 1].focus()
        }
        return
      case "Enter":
        if (inFilter && matches.length > 0) {
          e.preventDefault()
          goAndFocus(childPath(b.path, matches[0].name))
        }
        return
      case "ArrowRight":
        if (i >= 0) {
          e.preventDefault()
          rows[i].click()
        }
        return
      case "ArrowLeft":
      case "Backspace":
        if (inFilter && (e.key === "ArrowLeft" || b.filter !== "")) return
        e.preventDefault()
        upAndFocus()
        return
      default:
        if (!inFilter && e.key.length === 1 && e.key !== " " && ready) filterRef.current?.focus()
    }
  }

  const list = (() => {
    switch (b.listing.kind) {
      case "loading":
        return (
          // Drawn only after --picker-skeleton-delay, so fast folders never flicker.
          <div className="animate-in fill-mode-both delay-(--picker-skeleton-delay) duration-0 fade-in" role="status">
            <div className="flex items-center gap-2 px-3 pt-1.5 pb-0.5 text-caption text-muted-foreground">
              <span className="size-3.5 animate-spin rounded-full border-2 border-brand-150 border-t-primary" aria-hidden="true" />
              Reading {here}…
            </div>
            {[46, 62, 38, 54, 30].map((w) => (
              <div key={w} className="grid h-10 grid-cols-[20px_1fr] items-center gap-3 px-3" aria-hidden="true">
                <i className="block h-4 w-[18px] animate-pulse rounded-xs bg-grey-94" />
                <i className="block h-3 animate-pulse rounded-xs bg-grey-94" style={{ width: `${w}%` }} />
              </div>
            ))}
          </div>
        )
      case "denied":
        return (
          <Panel
            alert
            tone="error"
            icon={<LockClosedIcon />}
            title={`Can’t list the contents of ${here}`}
            actions={
              <>
                <UpButton b={b} label={`Up to ${parentName}`} />
                <RetryButton b={b} />
              </>
            }
          >
            <span className="text-caption break-all">{b.listing.message}</span>
            <span className="text-grey-26">
              You can still start here. If the host can’t enter the folder either, the new session reports that error when it starts.
            </span>
          </Panel>
        )
      case "error":
        return (
          <Panel alert tone="error" icon={<ErrorCircleIcon />} title={`Can’t list ${here}`} actions={<RetryButton b={b} />}>
            <span className="break-all">{b.listing.message}</span>
          </Panel>
        )
      case "gone":
        return (
          <Panel alert tone="error" icon={<ErrorCircleIcon />} title={`${here} no longer exists`} actions={<UpButton b={b} label={`Up to ${parentName}`} />}>
            <span>It was moved or deleted after the parent was listed.</span>
          </Panel>
        )
      case "ready":
        if (pool.length === 0 && query === "") {
          return (
            <Panel icon={<FolderOpenIcon />} title={`No folders inside ${here}`} actions={<UpButton b={b} label={`Up to ${parentName}`} />}>
              <span>It may still contain files. Start here, or go back up.</span>
            </Panel>
          )
        }
        if (matches.length === 0) {
          return (
            <Panel
              icon={<SearchIcon />}
              title={`No folder named “${b.filter}” here`}
              actions={
                <>
                  {hiddenMatches > 0 && (
                    <Button variant="outline" className={RECOVERY_BTN} onClick={b.toggleHidden}>
                      <EyeIcon className="size-4" />
                      Include hidden folders
                    </Button>
                  )}
                  <Button variant="ghost" className={`${RECOVERY_BTN} border-transparent bg-transparent hover:bg-foreground/5`} onClick={() => b.setFilter("")}>
                    <DismissIcon className="size-4" />
                    Clear filter
                  </Button>
                </>
              }
            >
              {hiddenMatches > 0 && <span>{plural(hiddenMatches, "hidden folder")} here match.</span>}
            </Panel>
          )
        }
        return (
          <ul className="m-0 list-none p-0" aria-label={`Folders in ${here}`}>
            {matches.map((e) => (
              <li key={e.name}>
                <button
                  type="button"
                  data-folder-row
                  title={e.name}
                  onClick={() => goAndFocus(childPath(b.path, e.name))}
                  className={`grid h-(--picker-row-h) w-full grid-cols-[20px_1fr_16px] items-center gap-3 rounded-sm pr-2.5 pl-3 text-left text-foreground hover:bg-grey-96 active:bg-grey-92 max-md:h-(--picker-row-h-touch) ${FOCUS} ${TRANSITION}`}
                >
                  <FolderIcon className="size-5 text-grey-38" />
                  <span
                    className={`truncate max-md:line-clamp-2 max-md:leading-[18px] max-md:break-all max-md:whitespace-normal ${e.hidden ? "text-grey-38" : ""}`}
                  >
                    <Highlight name={e.name} query={query} />
                  </span>
                  <ChevronRightIcon className="size-4 text-grey-74" />
                </button>
              </li>
            ))}
          </ul>
        )
    }
  })()

  const drillAnim =
    b.drill === "in"
      ? "animate-in fade-in slide-in-from-right-6 duration-(--picker-drill-dur) ease-(--picker-drill-ease) motion-reduce:animate-none"
      : b.drill === "up"
        ? "animate-in fade-in slide-in-from-left-6 duration-(--picker-drill-dur) ease-(--picker-drill-ease) motion-reduce:animate-none"
        : ""

  const foot = (() => {
    if (!ready) return null
    if (query !== "" && matches.length > 0) {
      return (
        <>
          <span>
            Enter opens <b className="text-foreground">{matches[0].name}</b>
          </span>
          <span className="max-md:hidden">
            <kbd className={KBD}>↑</kbd> <kbd className={KBD}>↓</kbd> move
          </span>
        </>
      )
    }
    if (hiddenEntries.length === 0 || query !== "") return null
    const names = hiddenEntries.slice(0, HIDDEN_NAMES_SHOWN).map((e) => e.name).join(", ")
    const more = hiddenEntries.length > HIDDEN_NAMES_SHOWN ? ", …" : ""
    return (
      <>
        <span className="min-w-0 truncate">
          {showHidden ? `Showing ${plural(hiddenEntries.length, "hidden folder")}` : plural(hiddenEntries.length, "hidden folder")}
          {!showHidden && <span className="max-md:hidden"> ({names}{more})</span>}
        </span>
        {!autoHidden && (
          <Button variant="ghost" className="h-7 gap-1.5 rounded-sm px-2.5 text-caption font-semibold hover:bg-foreground/5 max-md:h-11" onClick={b.toggleHidden}>
            {showHidden ? <EyeOffIcon className="size-4" /> : <EyeIcon className="size-4" />}
            {showHidden ? "Hide" : "Show"}
          </Button>
        )}
      </>
    )
  })()

  return (
    <div className={`flex flex-col overflow-hidden rounded-md border border-grey-88 bg-card ${className}`} onKeyDown={onKeyDown}>
      <div className="flex min-h-11 items-center gap-0.5 border-b border-grey-94 bg-grey-98 p-1 max-md:min-h-[52px]">
        <Crumbs b={b} />
      </div>
      <label
        className={`flex h-(--picker-filter-h) shrink-0 items-center gap-2 border-b border-grey-94 px-3 text-grey-38 focus-within:shadow-[inset_0_-2px_0_var(--primary)] max-md:h-(--picker-filter-h-touch) ${query !== "" ? "shadow-[inset_0_-2px_0_var(--primary)]" : ""}`}
      >
        <SearchIcon className="size-4 shrink-0" />
        <input
          ref={filterRef}
          type="search"
          aria-label="Filter folders"
          autoCapitalize="off"
          autoCorrect="off"
          spellCheck={false}
          enterKeyHint="go"
          disabled={!ready}
          placeholder={ready ? `Filter ${plural(pool.length, "folder")}` : "Filter folders"}
          value={b.filter}
          onChange={(e) => b.setFilter(e.target.value)}
          className="h-full min-w-0 flex-1 border-0 bg-transparent text-foreground outline-none placeholder:text-grey-38 [&::-webkit-search-cancel-button]:hidden"
        />
        {query !== "" && (
          <>
            <span className="text-caption whitespace-nowrap text-muted-foreground" aria-live="polite">
              {matches.length} of {pool.length}
            </span>
            <Button variant="ghost" className="size-7 rounded-sm text-grey-26 hover:bg-foreground/5 max-md:size-11" aria-label="Clear filter" onClick={() => b.setFilter("")}>
              <DismissIcon className="size-4" />
            </Button>
          </>
        )}
      </label>
      <div
        ref={listRef}
        tabIndex={-1}
        className="h-(--picker-list-h) overflow-y-auto overscroll-contain p-1 outline-none [scrollbar-width:thin] max-md:h-auto max-md:min-h-0 max-md:flex-1"
      >
        <div key={`${b.path}|${b.listing.kind}`} className={`flex min-h-full flex-col ${drillAnim}`}>
          {b.notice !== null && (
            <div className="mx-1 mt-1 mb-0.5 flex items-start gap-2.5 rounded-sm bg-warning-bg px-2.5 py-2 text-caption text-grey-26 inset-ring inset-ring-warning-border" role="status">
              <WarningIcon className="size-4 shrink-0 text-warning" />
              <span>
                <span className="font-mono text-foreground">{displayPath(b.notice)}</span> was moved or deleted. Opened its nearest parent instead.
              </span>
            </div>
          )}
          {list}
        </div>
      </div>
      {foot !== null && (
        <div className="flex items-center justify-between gap-2 border-t border-grey-94 py-1.5 pr-1.5 pl-3.5 text-caption text-muted-foreground">{foot}</div>
      )}
    </div>
  )
}

// "Starts in" line: the session's folder, truncated from the left so the folder name stays visible.
export function StartsIn({ b, className = "" }: { b: FolderBrowser; className?: string }) {
  const tone = b.listing.kind === "gone" ? "bad" : b.listing.kind === "denied" ? "warn" : "ok"
  return (
    <div className={`flex min-w-0 items-center gap-2 text-caption whitespace-nowrap ${tone === "warn" ? "text-warning" : "text-muted-foreground"} ${className}`}>
      {tone === "bad" ? (
        <ErrorCircleIcon className="size-4 shrink-0" />
      ) : tone === "warn" ? (
        <WarningIcon className="size-4 shrink-0" />
      ) : (
        <FolderOpenIcon className="size-4 shrink-0" />
      )}
      {tone === "bad" ? "Can’t start in" : tone === "warn" ? "Starts in (contents unlisted)" : "Starts in"}
      <code
        className={`min-w-0 truncate text-left font-mono [direction:rtl] ${tone === "bad" ? "text-destructive" : tone === "warn" ? "text-warning" : "text-foreground"}`}
      >
        <span dir="ltr">{displayPath(b.path)}</span>
      </code>
    </div>
  )
}
