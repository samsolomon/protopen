import { useEffect, useState } from 'react'
import type { AdminUser } from './api'
import { fetchAdminUsers, adminUpdatePlan, adminDeleteUser, SessionExpiredError } from './api'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Badge } from '@/components/ui/badge'

type AdminPanelProps = {
  onSessionExpired: () => void
}

export function AdminPanel({ onSessionExpired }: AdminPanelProps) {
  const [users, setUsers] = useState<AdminUser[]>([])
  const [loading, setLoading] = useState(true)
  const [deleteTarget, setDeleteTarget] = useState<AdminUser | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    void loadUsers()
  }, [])

  const loadUsers = async () => {
    try {
      setLoading(true)
      const data = await fetchAdminUsers()
      setUsers(data)
      setError(null)
    } catch (err) {
      if (err instanceof SessionExpiredError) {
        onSessionExpired()
        return
      }
      setError(err instanceof Error ? err.message : 'Could not load users')
    } finally {
      setLoading(false)
    }
  }

  const changePlan = async (user: AdminUser, plan: string) => {
    if (!user.orgId) return
    try {
      await adminUpdatePlan(user.orgId, plan)
      setUsers((prev) => prev.map((u) => (u.id === user.id ? { ...u, plan } : u)))
    } catch (err) {
      if (err instanceof SessionExpiredError) {
        onSessionExpired()
        return
      }
      setError(err instanceof Error ? err.message : 'Could not update plan')
    }
  }

  const confirmDelete = async () => {
    if (!deleteTarget) return
    try {
      await adminDeleteUser(deleteTarget.id)
      setUsers((prev) => prev.filter((u) => u.id !== deleteTarget.id))
      setDeleteTarget(null)
    } catch (err) {
      if (err instanceof SessionExpiredError) {
        onSessionExpired()
        return
      }
      setError(err instanceof Error ? err.message : 'Could not delete user')
    }
  }

  if (loading) {
    return <p className="text-sm text-muted-foreground">Loading users...</p>
  }

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h2 className="text-lg font-semibold tracking-tight">Users</h2>
        <p className="text-sm text-muted-foreground">{users.length} total</p>
      </div>

      {error && <p className="text-sm text-destructive">{error}</p>}

      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Email</TableHead>
            <TableHead>Name</TableHead>
            <TableHead>Plan</TableHead>
            <TableHead>Joined</TableHead>
            <TableHead className="w-24"></TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {users.map((user) => (
            <TableRow key={user.id}>
              <TableCell className="font-medium">{user.email}</TableCell>
              <TableCell>{user.name}</TableCell>
              <TableCell>
                {user.orgId ? (
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button variant="ghost" size="sm" className="h-6 px-2 gap-1">
                        <Badge variant="secondary" className="cursor-pointer">
                          {user.plan ?? 'tinkerer'}
                        </Badge>
                        <svg className="size-3 text-muted-foreground" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><polyline points="6 9 12 15 18 9"/></svg>
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent>
                      {['tinkerer', 'pro', 'team'].map((plan) => (
                        <DropdownMenuItem
                          key={plan}
                          onClick={() => void changePlan(user, plan)}
                          className={user.plan === plan ? 'font-medium' : ''}
                        >
                          {plan}
                        </DropdownMenuItem>
                      ))}
                    </DropdownMenuContent>
                  </DropdownMenu>
                ) : (
                  <span className="text-muted-foreground text-xs">—</span>
                )}
              </TableCell>
              <TableCell className="text-muted-foreground">{user.createdAt}</TableCell>
              <TableCell>
                <Button
                  variant="ghost"
                  size="sm"
                  className="text-destructive hover:text-destructive"
                  onClick={() => setDeleteTarget(user)}
                >
                  Delete
                </Button>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>

      <Dialog open={!!deleteTarget} onOpenChange={() => setDeleteTarget(null)}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Delete user</DialogTitle>
            <DialogDescription>
              This will permanently delete <strong>{deleteTarget?.email}</strong> and all their sites. This cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setDeleteTarget(null)}>Cancel</Button>
            <Button variant="destructive" onClick={() => void confirmDelete()}>Delete</Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  )
}
