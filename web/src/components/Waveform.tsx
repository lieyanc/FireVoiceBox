import { useCallback, useEffect, useRef, useState } from 'react'
import WaveSurfer from 'wavesurfer.js'
import { Play, Pause, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { formatDuration } from '@/lib/format'
import { cn } from '@/lib/utils'

interface WaveformProps {
  url: string
  height?: number
  className?: string
}

// Waveform renders a wavesurfer player. To avoid spinning up hundreds of audio
// decoders on a busy admin page, the instance is created lazily on first play.
export function Waveform({ url, height = 48, className }: WaveformProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const wsRef = useRef<WaveSurfer | null>(null)
  const playOnReadyRef = useRef(false)
  const [loading, setLoading] = useState(false)
  const [ready, setReady] = useState(false)
  const [playing, setPlaying] = useState(false)
  const [current, setCurrent] = useState(0)
  const [duration, setDuration] = useState(0)
  const [errored, setErrored] = useState(false)

  const ensureInstance = useCallback(() => {
    if (wsRef.current || !containerRef.current) return wsRef.current
    setLoading(true)
    const ws = WaveSurfer.create({
      container: containerRef.current,
      height,
      waveColor: 'rgba(161,161,170,0.45)',
      progressColor: 'rgba(244,244,245,0.95)',
      cursorColor: 'transparent',
      barWidth: 2,
      barGap: 1,
      barRadius: 2,
      normalize: true,
      url,
    })
    wsRef.current = ws
    ws.on('ready', () => {
      setReady(true)
      setLoading(false)
      setDuration(ws.getDuration())
      if (playOnReadyRef.current) {
        playOnReadyRef.current = false
        void ws.play()
      }
    })
    ws.on('play', () => setPlaying(true))
    ws.on('pause', () => setPlaying(false))
    ws.on('finish', () => setPlaying(false))
    ws.on('timeupdate', (t: number) => setCurrent(t))
    ws.on('error', () => {
      setErrored(true)
      setLoading(false)
    })
    return ws
  }, [url, height])

  useEffect(() => {
    return () => {
      wsRef.current?.destroy()
      wsRef.current = null
    }
  }, [])

  const toggle = useCallback(() => {
    const ws = wsRef.current
    if (!ws) {
      playOnReadyRef.current = true
      ensureInstance()
      return
    }
    if (ws.isPlaying()) ws.pause()
    else void ws.play()
  }, [ensureInstance])

  return (
    <div className={cn('flex items-center gap-3', className)}>
      <Button
        type="button"
        size="icon"
        variant="secondary"
        onClick={toggle}
        disabled={errored}
        className="shrink-0 rounded-full"
        aria-label={playing ? '暂停' : '播放'}
      >
        {loading ? (
          <Loader2 className="h-4 w-4 animate-spin" />
        ) : playing ? (
          <Pause className="h-4 w-4" />
        ) : (
          <Play className="h-4 w-4" />
        )}
      </Button>
      <div className="min-w-0 flex-1">
        <div ref={containerRef} className="w-full" style={{ minHeight: height }} />
        {errored && <p className="text-xs text-destructive">音频加载失败</p>}
      </div>
      {ready && (
        <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
          {formatDuration(current)} / {formatDuration(duration)}
        </span>
      )}
    </div>
  )
}
