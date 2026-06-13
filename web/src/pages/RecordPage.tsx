import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { useParams } from 'react-router-dom'
import { Mic, Square, RotateCcw, Upload, CheckCircle2, Loader2, AlertTriangle } from 'lucide-react'
import { api, ApiError, type PublicProject } from '@/lib/api'
import { useRecorder } from '@/hooks/useRecorder'
import { Waveform } from '@/components/Waveform'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { useToast } from '@/components/ui/toast'
import { AppFooter } from '@/components/AppFooter'
import { formatDuration } from '@/lib/format'

function extForMime(mime: string): string {
  if (mime.includes('webm')) return '.webm'
  if (mime.includes('mp4')) return '.mp4'
  if (mime.includes('mpeg')) return '.mp3'
  if (mime.includes('ogg')) return '.ogg'
  return '.bin'
}

type Step = 'form' | 'record' | 'done'

export function RecordPage() {
  const { key = '' } = useParams()
  const { error: toastError } = useToast()
  const [project, setProject] = useState<PublicProject | null>(null)
  const [loadError, setLoadError] = useState('')
  const [loading, setLoading] = useState(true)

  const [step, setStep] = useState<Step>('form')
  const [studentId, setStudentId] = useState('')
  const [nickname, setNickname] = useState('')
  const [uploading, setUploading] = useState(false)

  const maxDuration = project?.max_duration_sec ?? 60
  const recorder = useRecorder({ maxDurationSec: maxDuration })

  useEffect(() => {
    let cancelled = false
    api
      .getPublicProject(key)
      .then((p) => {
        if (!cancelled) setProject(p)
      })
      .catch((e: unknown) => {
        if (!cancelled) setLoadError(e instanceof ApiError ? e.message : '加载失败')
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [key])

  const progress = useMemo(() => {
    if (maxDuration <= 0) return 0
    return Math.min(100, (recorder.elapsed / maxDuration) * 100)
  }, [recorder.elapsed, maxDuration])

  async function handleUpload() {
    if (!recorder.recording || !project) return
    setUploading(true)
    try {
      const form = new FormData()
      form.append('student_id', studentId.trim())
      form.append('nickname', nickname.trim())
      form.append('duration_sec', String(Math.round(recorder.recording.durationSec)))
      form.append('mime', recorder.recording.mimeType)
      form.append('audio', recorder.recording.blob, 'recording' + extForMime(recorder.recording.mimeType))
      await api.submit(project.id, form)
      setStep('done')
    } catch (e) {
      toastError(e instanceof ApiError ? e.message : '上传失败，请重试')
    } finally {
      setUploading(false)
    }
  }

  if (loading) {
    return (
      <Centered>
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </Centered>
    )
  }

  if (loadError || !project) {
    return (
      <Centered>
        <Card className="w-full max-w-md">
          <CardHeader className="items-center text-center">
            <AlertTriangle className="mb-2 h-10 w-10 text-destructive" />
            <CardTitle>无法打开</CardTitle>
            <CardDescription>{loadError || '项目不存在'}</CardDescription>
          </CardHeader>
        </Card>
      </Centered>
    )
  }

  return (
    <div className="mx-auto flex min-h-screen w-full max-w-md flex-col px-5 py-10">
      <header className="mb-8 text-center">
        <h1 className="text-2xl font-bold tracking-tight">{project.title}</h1>
        {project.description && (
          <p className="mt-2 whitespace-pre-wrap text-sm text-muted-foreground">{project.description}</p>
        )}
      </header>

      <main className="flex-1">
        {!project.accepting ? (
          <Card>
            <CardHeader className="items-center text-center">
              <CardTitle>暂未开放</CardTitle>
              <CardDescription>该祝福收集已关闭，感谢你的关注。</CardDescription>
            </CardHeader>
          </Card>
        ) : step === 'done' ? (
          <Card>
            <CardContent className="flex flex-col items-center gap-3 py-10 text-center">
              <CheckCircle2 className="h-14 w-14 text-emerald-500" />
              <h2 className="text-xl font-semibold">提交成功！</h2>
              <p className="text-sm text-muted-foreground">你的祝福已经收到，谢谢 ❤️</p>
            </CardContent>
          </Card>
        ) : step === 'form' ? (
          <Card>
            <CardHeader>
              <CardTitle className="text-lg">填写信息</CardTitle>
              <CardDescription>录制前请填写你的学号与昵称</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="student">学号</Label>
                <Input
                  id="student"
                  value={studentId}
                  onChange={(e) => setStudentId(e.target.value)}
                  placeholder="例如 23xxxx"
                  maxLength={64}
                  inputMode="text"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="nick">昵称</Label>
                <Input
                  id="nick"
                  value={nickname}
                  onChange={(e) => setNickname(e.target.value)}
                  placeholder="希望大家怎么称呼你"
                  maxLength={64}
                />
              </div>
              <Button
                className="w-full"
                size="lg"
                disabled={!studentId.trim() || !nickname.trim()}
                onClick={() => setStep('record')}
              >
                下一步：录制祝福
              </Button>
              <p className="text-center text-xs text-muted-foreground">
                单条最长 {formatDuration(maxDuration)}
              </p>
            </CardContent>
          </Card>
        ) : (
          <Card>
            <CardHeader>
              <CardTitle className="text-lg">录制祝福</CardTitle>
              <CardDescription>
                {nickname}（{studentId}）· 最长 {formatDuration(maxDuration)}
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              {/* Timer + progress */}
              <div className="flex flex-col items-center gap-3">
                <div className="text-5xl font-bold tabular-nums">
                  {formatDuration(recorder.elapsed)}
                  <span className="text-xl text-muted-foreground"> / {formatDuration(maxDuration)}</span>
                </div>
                <div className="h-2 w-full overflow-hidden rounded-full bg-secondary">
                  <div
                    className="h-full rounded-full bg-primary transition-[width] duration-200"
                    style={{ width: `${progress}%` }}
                  />
                </div>
              </div>

              {/* Controls per state */}
              {recorder.state === 'recorded' && recorder.recording ? (
                <div className="space-y-4">
                  <div className="rounded-lg border bg-background p-3">
                    <p className="mb-2 text-xs text-muted-foreground">重听一下：</p>
                    <Waveform url={recorder.recording.url} />
                  </div>
                  <div className="grid grid-cols-2 gap-3">
                    <Button variant="outline" onClick={recorder.reset} disabled={uploading}>
                      <RotateCcw className="h-4 w-4" /> 重录
                    </Button>
                    <Button onClick={handleUpload} disabled={uploading}>
                      {uploading ? <Loader2 className="h-4 w-4 animate-spin" /> : <Upload className="h-4 w-4" />}
                      {uploading ? '上传中' : '上传'}
                    </Button>
                  </div>
                </div>
              ) : recorder.state === 'recording' ? (
                <div className="flex flex-col items-center gap-4">
                  <div className="flex items-center gap-2 text-sm text-destructive">
                    <span className="h-3 w-3 animate-pulse-rec rounded-full bg-destructive" /> 正在录制
                  </div>
                  <Button size="xl" variant="destructive" className="rounded-full" onClick={recorder.stop}>
                    <Square className="h-5 w-5" /> 停止
                  </Button>
                </div>
              ) : recorder.state === 'requesting' ? (
                <div className="flex flex-col items-center gap-3 py-4 text-muted-foreground">
                  <Loader2 className="h-6 w-6 animate-spin" />
                  <p className="text-sm">正在请求麦克风权限…</p>
                </div>
              ) : (
                <div className="flex flex-col items-center gap-4">
                  {(recorder.state === 'denied' || recorder.state === 'error') && (
                    <p className="text-center text-sm text-destructive">{recorder.error}</p>
                  )}
                  <Button size="xl" className="rounded-full" onClick={recorder.start}>
                    <Mic className="h-5 w-5" /> 开始录制
                  </Button>
                  <button
                    type="button"
                    className="text-xs text-muted-foreground underline-offset-2 hover:underline"
                    onClick={() => setStep('form')}
                  >
                    返回修改信息
                  </button>
                </div>
              )}
            </CardContent>
          </Card>
        )}
      </main>

      <AppFooter className="mt-8" />
    </div>
  )
}

function Centered({ children }: { children: ReactNode }) {
  return <div className="flex min-h-screen items-center justify-center p-6">{children}</div>
}
