import { useCallback, useEffect, useMemo, useState } from 'react'
import { CheckCircle2, Download, RefreshCw, RotateCw, X } from 'lucide-react'
import { api, ApiError, type UpdateCheckResult, type UpdateStatus, type VersionInfo } from '@/lib/api'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { refreshIfClientCacheChanged } from '@/lib/client-cache'

const ACTIVE_STATES = new Set(['checking', 'downloading', 'applying'])

function stateLabel(state: string) {
  switch (state) {
    case 'checking':
      return '检查中'
    case 'downloading':
      return '下载中'
    case 'ready':
      return '待应用'
    case 'applying':
      return '重启中'
    case 'failed':
      return '失败'
    default:
      return '空闲'
  }
}

function stateVariant(state: string) {
  if (state === 'ready') return 'success' as const
  if (state === 'failed') return 'destructive' as const
  if (state === 'applying' || ACTIVE_STATES.has(state)) return 'secondary' as const
  return 'outline' as const
}

function percent(value: number | undefined) {
  if (!Number.isFinite(value)) return 0
  return Math.max(0, Math.min(100, Math.round(value || 0)))
}

function messageOf(err: unknown, fallback: string) {
  if (err instanceof ApiError || err instanceof Error) return err.message
  return fallback
}

function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border border-border/70 px-3 py-2">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="mt-1 break-all font-mono text-sm tabular-nums">{value || '--'}</div>
    </div>
  )
}

export function UpdatePanel() {
  const [version, setVersion] = useState<VersionInfo | null>(null)
  const [status, setStatus] = useState<UpdateStatus | null>(null)
  const [checkResult, setCheckResult] = useState<UpdateCheckResult | null>(null)
  const [message, setMessage] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const load = useCallback(async () => {
    const [versionData, statusData, clientVersion] = await Promise.all([
      api.version(true),
      api.updateStatus(),
      api.clientVersion(true),
    ])
    if (
      await refreshIfClientCacheChanged(clientVersion.cache_key, () => {
        setVersion(versionData.version)
        setStatus(statusData.status)
        setMessage('新版本已安装，正在强制刷新缓存')
      })
    ) {
      return
    }
    setVersion(versionData.version)
    setStatus(statusData.status)
  }, [])

  useEffect(() => {
    load().catch((err) => setMessage(messageOf(err, '加载失败')))
  }, [load])

  useEffect(() => {
    if (!status || (!ACTIVE_STATES.has(String(status.state)) && status.state !== 'ready')) return
    const timer = window.setInterval(() => {
      load().catch(() => setMessage('服务可能正在重启'))
    }, 3000)
    return () => window.clearInterval(timer)
  }, [load, status])

  const active = ACTIVE_STATES.has(String(status?.state || 'idle'))
  const ready = status?.state === 'ready'
  const progress = percent(status?.progress)
  const latest = status?.latest_version || checkResult?.latest_version || ''
  const notes = status?.release_notes || checkResult?.release_notes || ''

  const summary = useMemo(() => {
    if (!status) return '加载中'
    if (status.state === 'failed') return status.error || '更新失败'
    if (status.state === 'ready') return latest ? `新版本 ${latest} 已下载` : '更新包已下载'
    if (active) return `${stateLabel(String(status.state))} ${progress}%`
    if (checkResult?.has_update) return `发现新版本 ${checkResult.latest_version || ''}`.trim()
    return '已就绪'
  }, [active, checkResult, latest, progress, status])

  async function check() {
    setBusy(true)
    setMessage(null)
    try {
      const data = await api.checkUpdate()
      setCheckResult(data.result)
      setMessage(
        data.ok
          ? data.result.has_update
            ? `发现新版本 ${data.result.latest_version || ''}`.trim()
            : '当前已是最新版本'
          : `检查失败：${data.error || '未知错误'}`,
      )
      await load()
    } catch (err) {
      setMessage(messageOf(err, '检查失败'))
    } finally {
      setBusy(false)
    }
  }

  async function apply() {
    setBusy(true)
    setMessage(null)
    try {
      await api.applyUpdate()
      setMessage(ready ? '正在应用更新' : '更新任务已启动')
      await load().catch(() => setMessage('服务可能正在重启'))
    } catch (err) {
      setMessage(messageOf(err, '更新失败'))
    } finally {
      setBusy(false)
    }
  }

  async function dismiss() {
    setBusy(true)
    try {
      await api.dismissUpdate()
      setCheckResult(null)
      setMessage('已忽略当前更新')
      await load()
    } catch (err) {
      setMessage(messageOf(err, '操作失败'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader className="flex-row items-start justify-between gap-4 space-y-0">
          <div className="min-w-0">
            <CardTitle>系统更新</CardTitle>
            <p className="mt-1 text-sm text-muted-foreground">{summary}</p>
          </div>
          <Badge variant={stateVariant(String(status?.state || 'idle'))} className="shrink-0 gap-1.5">
            {status?.state === 'idle' && !checkResult?.has_update ? <CheckCircle2 className="h-3.5 w-3.5" /> : null}
            {stateLabel(String(status?.state || 'idle'))}
          </Badge>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-3 sm:grid-cols-2">
            <InfoRow label="当前版本" value={status?.current_version || version?.version || ''} />
            <InfoRow label="最新版本" value={latest} />
            <InfoRow label="Commit" value={version?.commit || ''} />
            <InfoRow label="构建时间" value={version?.build_time || ''} />
            <InfoRow label="通道" value={version?.update_channel || checkResult?.channel || ''} />
            <InfoRow label="仓库" value={version?.update_repo || ''} />
          </div>

          {(active || ready) && (
            <div className="space-y-2">
              <div className="flex items-center justify-between text-xs text-muted-foreground">
                <span>{stateLabel(String(status?.state || 'idle'))}</span>
                <span className="font-mono tabular-nums">{progress}%</span>
              </div>
              <div className="h-2 overflow-hidden rounded-full bg-muted">
                <div className="h-full rounded-full bg-primary transition-[width]" style={{ width: `${progress}%` }} />
              </div>
            </div>
          )}

          {message && (
            <Badge variant={message.includes('失败') || message.includes('error') ? 'destructive' : 'secondary'}>
              {message}
            </Badge>
          )}

          {notes && (
            <div className="max-h-56 overflow-auto whitespace-pre-wrap rounded-md border border-border/70 bg-muted/30 p-3 text-sm">
              {notes}
            </div>
          )}

          <div className="flex flex-wrap justify-end gap-2">
            {ready && (
              <Button variant="outline" disabled={busy || active} onClick={dismiss}>
                <X className="h-4 w-4" /> 忽略
              </Button>
            )}
            <Button variant="outline" disabled={busy || active} onClick={check}>
              <RefreshCw className="h-4 w-4" /> 检查更新
            </Button>
            <Button disabled={busy || active} onClick={apply}>
              {ready ? <RotateCw className="h-4 w-4" /> : <Download className="h-4 w-4" />}
              {ready ? '应用更新' : '下载并更新'}
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
