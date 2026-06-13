import { useEffect, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { Plus, LogOut, Loader2, Mic, Lock, RefreshCw } from 'lucide-react'
import { api, ApiError, type Project } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { useToast } from '@/components/ui/toast'
import { ProjectFormDialog } from '@/components/ProjectFormDialog'
import { formatDateTime, formatDuration } from '@/lib/format'
import { UpdatePanel } from '@/pages/UpdatePanel'

export function AdminHome() {
  const [authed, setAuthed] = useState<boolean | null>(null)

  useEffect(() => {
    api
      .me()
      .then(() => setAuthed(true))
      .catch(() => setAuthed(false))
  }, [])

  if (authed === null) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }
  return authed ? <ProjectList onLogout={() => setAuthed(false)} /> : <LoginForm onSuccess={() => setAuthed(true)} />
}

function LoginForm({ onSuccess }: { onSuccess: () => void }) {
  const { error } = useToast()
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    try {
      await api.login(password)
      onSuccess()
    } catch (err) {
      error(err instanceof ApiError ? err.message : '登录失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center p-6">
      <Card className="w-full max-w-sm">
        <CardHeader className="items-center text-center">
          <div className="mb-2 flex h-12 w-12 items-center justify-center rounded-full bg-secondary">
            <Lock className="h-6 w-6" />
          </div>
          <CardTitle>管理员登录</CardTitle>
          <CardDescription>FireVoiceBox 后台</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={submit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="pw">密码</Label>
              <Input
                id="pw"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoFocus
              />
            </div>
            <Button type="submit" className="w-full" disabled={busy || !password}>
              {busy && <Loader2 className="h-4 w-4 animate-spin" />}
              登录
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}

function ProjectList({ onLogout }: { onLogout: () => void }) {
  const { error, success } = useToast()
  const [projects, setProjects] = useState<Project[] | null>(null)
  const [createOpen, setCreateOpen] = useState(false)
  const [view, setView] = useState<'projects' | 'update'>('projects')

  async function load() {
    try {
      setProjects(await api.listProjects())
    } catch (e) {
      error(e instanceof ApiError ? e.message : '加载失败')
    }
  }

  useEffect(() => {
    void load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function handleCreate(values: Partial<Project>) {
    try {
      await api.createProject(values)
      success('项目已创建')
      await load()
    } catch (e) {
      error(e instanceof ApiError ? e.message : '创建失败')
      throw e
    }
  }

  async function logout() {
    await api.logout().catch(() => {})
    onLogout()
  }

  return (
    <div className="container py-8">
      <header className="mb-6 flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap items-center gap-3">
          <div className="flex items-center gap-2">
            <Mic className="h-6 w-6" />
            <h1 className="text-xl font-bold">FireVoiceBox</h1>
          </div>
          <div className="flex items-center gap-1 rounded-md border border-border p-1">
            <Button
              size="sm"
              variant={view === 'projects' ? 'secondary' : 'ghost'}
              onClick={() => setView('projects')}
            >
              <Mic className="h-4 w-4" /> 项目
            </Button>
            <Button
              size="sm"
              variant={view === 'update' ? 'secondary' : 'ghost'}
              onClick={() => setView('update')}
            >
              <RefreshCw className="h-4 w-4" /> 更新
            </Button>
          </div>
        </div>
        <div className="flex items-center gap-2">
          {view === 'projects' && (
            <Button onClick={() => setCreateOpen(true)}>
              <Plus className="h-4 w-4" /> 新建项目
            </Button>
          )}
          <Button variant="ghost" size="icon" onClick={logout} aria-label="退出登录">
            <LogOut className="h-4 w-4" />
          </Button>
        </div>
      </header>

      {view === 'update' ? (
        <UpdatePanel />
      ) : (
        <>
          {projects === null ? (
            <div className="flex justify-center py-20">
              <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
            </div>
          ) : projects.length === 0 ? (
            <Card>
              <CardContent className="flex flex-col items-center gap-3 py-16 text-center">
                <p className="text-muted-foreground">还没有项目，点击右上角新建一个吧。</p>
                <Button onClick={() => setCreateOpen(true)}>
                  <Plus className="h-4 w-4" /> 新建项目
                </Button>
              </CardContent>
            </Card>
          ) : (
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {projects.map((p) => (
                <Link key={p.id} to={`/admin/projects/${p.id}`}>
                  <Card className="h-full transition-colors hover:border-primary/50">
                    <CardHeader>
                      <div className="flex items-center justify-between gap-2">
                        <CardTitle className="truncate text-lg">{p.title}</CardTitle>
                        <Badge variant={p.status === 'open' ? 'success' : 'secondary'}>
                          {p.status === 'open' ? '收集中' : '已关闭'}
                        </Badge>
                      </div>
                      {p.description && <CardDescription className="line-clamp-2">{p.description}</CardDescription>}
                    </CardHeader>
                    <CardContent className="flex items-center justify-between text-sm text-muted-foreground">
                      <span>{p.submission_count} 条投稿</span>
                      <span>≤ {formatDuration(p.max_duration_sec)}</span>
                    </CardContent>
                    <CardContent className="pt-0 text-xs text-muted-foreground">
                      创建于 {formatDateTime(p.created_at)}
                    </CardContent>
                  </Card>
                </Link>
              ))}
            </div>
          )}
        </>
      )}

      <ProjectFormDialog mode="create" open={createOpen} onOpenChange={setCreateOpen} onSubmit={handleCreate} />
    </div>
  )
}
