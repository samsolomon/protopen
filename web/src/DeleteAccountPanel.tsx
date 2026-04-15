import { useState } from 'react'
import { deleteAccount, SessionExpiredError } from './api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

type DeleteAccountPanelProps = {
  onAccountDeleted: () => void
  onSessionExpired: () => void
}

export function DeleteAccountPanel({ onAccountDeleted, onSessionExpired }: DeleteAccountPanelProps) {
  const [open, setOpen] = useState(false)
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [deleting, setDeleting] = useState(false)

  const handleOpen = (next: boolean) => {
    setOpen(next)
    if (!next) {
      setPassword('')
      setError(null)
    }
  }

  const handleDelete = async () => {
    setError(null)
    setDeleting(true)
    try {
      await deleteAccount(password)
      onAccountDeleted()
    } catch (err) {
      if (err instanceof SessionExpiredError) {
        onSessionExpired()
        return
      }
      setError(err instanceof Error ? err.message : 'Could not delete account')
    } finally {
      setDeleting(false)
    }
  }

  return (
    <section>
      <h3 className="mb-3 text-sm font-medium text-destructive">Danger zone</h3>
      <div className="rounded-lg border border-destructive/30 bg-card">
        <div className="flex items-center justify-between px-4 py-3">
          <div>
            <p className="text-sm">Delete account</p>
            <p className="text-sm text-muted-foreground">Permanently delete your account and all data.</p>
          </div>
          <Button variant="destructive" size="sm" onClick={() => handleOpen(true)}>
            Delete account
          </Button>
        </div>
      </div>

      <Dialog open={open} onOpenChange={handleOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Are you sure?</DialogTitle>
            <DialogDescription>
              This will permanently delete your account, projects, deploys, and API tokens. Enter
              your password to confirm.
            </DialogDescription>
          </DialogHeader>
          <div className="flex flex-col gap-2">
            <Label htmlFor="delete-password">Password</Label>
            <Input
              id="delete-password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && password) void handleDelete()
              }}
            />
            {error ? <p className="text-sm text-destructive">{error}</p> : null}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => handleOpen(false)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => void handleDelete()}
              disabled={deleting || !password}
            >
              {deleting ? 'Deleting...' : 'Delete my account'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  )
}
