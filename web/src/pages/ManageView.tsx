import { useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams, Link } from 'react-router-dom'
import {
  ArrowLeft,
  Loader2,
  Trash2,
  Download,
  Settings,
  Copy,
  Link as LinkIcon,
  Share2,
  AlertTriangle,
} from 'lucide-react'
import { api, ApiError, type Project, type Submission } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Waveform } from '@/components/Waveform'
import { ProjectFormDialog } from '@/components/ProjectFormDialog'
import { AppFooter } from '@/components/AppFooter'
import { useToast } from '@/components/ui/toast'
import { formatBytes, formatDateTime, formatDuration } from '@/lib/format'

function tokenFromHash(): string | undefined {
  const h = window.location.hash.replace(/^#/, '')
  const v = new URLSearchParams(h).get('key')
  return v || undefined
}

export function ManageView({ fromHash = false }: { fromHash?: boolean }) {
  const { id = '' } = useParams()
  const navigate = useNavigate()
  const { success, error } = useToast()
  const token = useMemo(() => (fromHash ? tokenFromHash() : undefined), [fromHash])
  const isOwner = !fromHash

  const [project, setProject] = useState<Project | null>(null)
  const [subs, setSubs] = useState<Submission[] | null>(null)
  const [loadError, setLoadError] = useState('')
  const [editOpen, setEditOpen] = useState(false)

  useEffect(() => {
    let cancelled = false
    Promise.all([api.getManageProject(id, token), api.listSubmissions(id, token)])
      .then(([p, list]) => {
        if (cancelled) return
        setProject(p)
        setSubs(list)
      })
      .catch((e: unknown) => {
        if (!cancelled) setLoadError(e instanceof ApiError ? e.message : '加载失败')
      })
    return () => {
      cancelled = true
    }
  }, [id, token])

  const origin = window.location.origin
  const recordUrl = project ? `${origin}/r/${project.slug || project.id}` : ''
  const shareUrl = project ? `${origin}/admin/p/${project.id}#key=${project.manage_token}` : ''

  async function copy(text: string, label: string) {
    try {
      await navigator.clipboard.writeText(text)
      success(`${label}已复制`)
    } catch {
      error('复制失败，请手动复制')
    }
  }

  async function handleEdit(values: Partial<Project>) {
    try {
      const updated = await api.updateProject(id, values)
      setProject(updated)
      success('已保存')
    } catch (e) {
      error(e instanceof ApiError ? e.message : '保存失败')
      throw e
    }
  }

  async function handleDeleteProject() {
    if (!window.confirm('确定删除该项目？所有投稿与音频将一并删除，且不可恢复。')) return
    try {
      await api.deleteProject(id)
      success('项目已删除')
      navigate('/admin')
    } catch (e) {
      error(e instanceof ApiError ? e.message : '删除失败')
    }
  }

  async function handleDeleteSub(sub: Submission) {
    if (!window.confirm(`删除 ${sub.nickname} 的投稿？`)) return
    try {
      await api.deleteSubmission(sub.id, token)
      setSubs((prev) => (prev ? prev.filter((s) => s.id !== sub.id) : prev))
      success('已删除')
    } catch (e) {
      error(e instanceof ApiError ? e.message : '删除失败')
    }
  }

  if (loadError) {
    return (
      <div className="flex min-h-screen items-center justify-center p-6">
        <Card className="w-full max-w-md">
          <CardHeader className="items-center text-center">
            <AlertTriangle className="mb-2 h-10 w-10 text-destructive" />
            <CardTitle>无法访问</CardTitle>
            <p className="mt-1 text-sm text-muted-foreground">{loadError}</p>
          </CardHeader>
        </Card>
      </div>
    )
  }

  if (!project || subs === null) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  return (
    <div className="container py-8">
      <header className="mb-6">
        {isOwner && (
          <Link to="/admin" className="mb-3 inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground">
            <ArrowLeft className="h-4 w-4" /> 返回项目列表
          </Link>
        )}
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-3">
            <h1 className="text-2xl font-bold">{project.title}</h1>
            <Badge variant={project.status === 'open' ? 'success' : 'secondary'}>
              {project.status === 'open' ? '收集中' : '已关闭'}
            </Badge>
          </div>
          <div className="flex items-center gap-2">
            <Button variant="outline" asChild>
              <a href={api.exportUrl(id, token)}>
                <Download className="h-4 w-4" /> 导出 ZIP
              </a>
            </Button>
            {isOwner && (
              <>
                <Button variant="outline" onClick={() => setEditOpen(true)}>
                  <Settings className="h-4 w-4" /> 设置
                </Button>
                <Button variant="ghost" size="icon" onClick={handleDeleteProject} aria-label="删除项目">
                  <Trash2 className="h-4 w-4" />
                </Button>
              </>
            )}
          </div>
        </div>
        {project.description && (
          <p className="mt-2 whitespace-pre-wrap text-sm text-muted-foreground">{project.description}</p>
        )}
      </header>

      {/* Share links */}
      <div className="mb-6 grid gap-3 sm:grid-cols-2">
        <Card>
          <CardContent className="flex items-center gap-3 py-4">
            <LinkIcon className="h-5 w-5 shrink-0 text-muted-foreground" />
            <div className="min-w-0 flex-1">
              <p className="text-xs text-muted-foreground">录制页链接（发给同学）</p>
              <p className="truncate text-sm">{recordUrl}</p>
            </div>
            <Button variant="ghost" size="icon" onClick={() => copy(recordUrl, '录制链接')}>
              <Copy className="h-4 w-4" />
            </Button>
          </CardContent>
        </Card>
        {isOwner && (
          <Card>
            <CardContent className="flex items-center gap-3 py-4">
              <Share2 className="h-5 w-5 shrink-0 text-muted-foreground" />
              <div className="min-w-0 flex-1">
                <p className="text-xs text-muted-foreground">协作管理链接（含密钥，发给协作者）</p>
                <p className="truncate text-sm">{shareUrl}</p>
              </div>
              <Button variant="ghost" size="icon" onClick={() => copy(shareUrl, '管理链接')}>
                <Copy className="h-4 w-4" />
              </Button>
            </CardContent>
          </Card>
        )}
      </div>

      {/* Stats */}
      <div className="mb-4 flex flex-wrap gap-4 text-sm text-muted-foreground">
        <span>共 {subs.length} 条投稿</span>
        <span>单条上限 {formatDuration(project.max_duration_sec)}</span>
        <span>单 IP 限 {project.max_per_ip === 0 ? '不限' : project.max_per_ip + ' 条'}</span>
      </div>

      {subs.length === 0 ? (
        <Card>
          <CardContent className="py-16 text-center text-muted-foreground">还没有投稿。</CardContent>
        </Card>
      ) : (
        <Card>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-[140px]">昵称 / 学号</TableHead>
                <TableHead className="min-w-[260px]">音频</TableHead>
                <TableHead className="w-[80px]">时长</TableHead>
                <TableHead className="w-[120px]">IP</TableHead>
                <TableHead className="w-[140px]">时间</TableHead>
                <TableHead className="w-[60px]" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {subs.map((s) => (
                <TableRow key={s.id}>
                  <TableCell>
                    <div className="font-medium">{s.nickname}</div>
                    <div className="text-xs text-muted-foreground">{s.student_id}</div>
                  </TableCell>
                  <TableCell>
                    <Waveform url={api.audioUrl(s.audio_path, token)} />
                    <div className="mt-1 text-[11px] text-muted-foreground">{formatBytes(s.size_bytes)}</div>
                  </TableCell>
                  <TableCell className="tabular-nums">{formatDuration(s.duration_sec)}</TableCell>
                  <TableCell className="font-mono text-xs">{s.ip}</TableCell>
                  <TableCell className="text-xs text-muted-foreground">{formatDateTime(s.created_at)}</TableCell>
                  <TableCell>
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => handleDeleteSub(s)}
                      aria-label="删除投稿"
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Card>
      )}

      <ProjectFormDialog
        mode="edit"
        open={editOpen}
        onOpenChange={setEditOpen}
        initial={project}
        onSubmit={handleEdit}
      />

      <AppFooter className="mt-10" />
    </div>
  )
}
