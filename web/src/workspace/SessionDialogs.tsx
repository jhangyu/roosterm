import { useState, type SubmitEvent } from "react"
import type { Tab } from "@/api/types"
import { DismissIcon } from "@/components/icons"
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { Dialog, DialogClose, DialogContent, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { BTN, BTN_DANGER, BTN_PRIMARY, DIALOG, FIELD_INPUT, FIELD_LABEL, ICON_BTN } from "@/components/classes"

// docs/api.md "Label: 1–64 characters" (PATCH /api/tabs/{id}): the host returns 400 past it.
const LABEL_MAX_CHARS = 64

// Not in mockup 3: rename dialog in the screen-03 dialog style (head row, field, right-aligned actions).
export function RenameDialog({ tab, onCancel, onRename }: { tab: Tab | null; onCancel: () => void; onRename: (id: string, label: string) => void }) {
  return (
    <Dialog open={tab !== null} onOpenChange={(o) => !o && onCancel()}>
      <DialogContent className={DIALOG}>
        {tab !== null && <RenameForm key={tab.id} tab={tab} onRename={onRename} />}
      </DialogContent>
    </Dialog>
  )
}

function RenameForm({ tab, onRename }: { tab: Tab; onRename: (id: string, label: string) => void }) {
  const [draft, setDraft] = useState(tab.label)
  const label = draft.trim()
  const submit = (ev: SubmitEvent) => {
    ev.preventDefault()
    onRename(tab.id, label)
  }
  return (
    <form onSubmit={submit}>
      <div className="mb-[22px] flex items-start justify-between">
        <DialogTitle className="text-subtitle1 font-semibold">Rename session</DialogTitle>
        <DialogClose asChild>
          <Button variant="ghost" className={ICON_BTN} aria-label="Close">
            <DismissIcon />
          </Button>
        </DialogClose>
      </div>
      <div className="mb-[26px] grid gap-1.5">
        <Label htmlFor="session-label" className={FIELD_LABEL}>Name</Label>
        <Input
          id="session-label"
          autoFocus
          maxLength={LABEL_MAX_CHARS}
          className={FIELD_INPUT}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          required
        />
      </div>
      <div className="flex justify-end gap-2">
        <DialogClose asChild>
          <Button type="button" variant="outline" className={BTN}>
            Cancel
          </Button>
        </DialogClose>
        <Button type="submit" className={BTN_PRIMARY} disabled={label === ""}>
          Save
        </Button>
      </div>
    </form>
  )
}

// Not in mockup 3: confirm before closing a running session, same dialog style, danger-outline confirm.
export function CloseConfirm({ tab, onCancel, onConfirm }: { tab: Tab | null; onCancel: () => void; onConfirm: (id: string) => void }) {
  return (
    <AlertDialog open={tab !== null} onOpenChange={(o) => !o && onCancel()}>
      <AlertDialogContent className={`${DIALOG} sm:max-w-[480px]`}>
        <AlertDialogTitle className="mb-1.5 text-subtitle1 font-semibold">Close “{tab?.label}”?</AlertDialogTitle>
        <AlertDialogDescription className="mb-[22px] text-body text-muted-foreground">
          Closing the session ends the program running in it.
        </AlertDialogDescription>
        <AlertDialogFooter className="mx-0 mb-0 flex-row justify-end gap-2 rounded-none border-0 bg-transparent p-0">
          <Button variant="outline" className={BTN} onClick={onCancel}>
            Cancel
          </Button>
          <Button variant="outline" className={BTN_DANGER} onClick={() => tab !== null && onConfirm(tab.id)}>
            Close session
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
