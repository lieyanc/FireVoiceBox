import { useCallback, useEffect, useRef, useState } from 'react'

export type RecorderState =
  | 'idle' // before permission
  | 'requesting' // asking for mic
  | 'recording'
  | 'recorded' // finished, blob ready for review
  | 'denied' // permission refused
  | 'error'

export interface Recording {
  blob: Blob
  url: string
  mimeType: string
  durationSec: number
}

const MIME_CANDIDATES = [
  'audio/webm;codecs=opus',
  'audio/webm',
  'audio/mp4',
  'audio/mpeg',
  'audio/ogg;codecs=opus',
]

function pickMimeType(): string {
  if (typeof MediaRecorder === 'undefined') return ''
  for (const t of MIME_CANDIDATES) {
    try {
      if (MediaRecorder.isTypeSupported(t)) return t
    } catch {
      // isTypeSupported may be absent on old browsers
    }
  }
  return ''
}

export interface UseRecorderOptions {
  maxDurationSec: number
}

export function useRecorder({ maxDurationSec }: UseRecorderOptions) {
  const [state, setState] = useState<RecorderState>('idle')
  const [elapsed, setElapsed] = useState(0)
  const [recording, setRecording] = useState<Recording | null>(null)
  const [error, setError] = useState<string>('')

  const recorderRef = useRef<MediaRecorder | null>(null)
  const streamRef = useRef<MediaStream | null>(null)
  const chunksRef = useRef<Blob[]>([])
  const timerRef = useRef<number | null>(null)
  const startedAtRef = useRef<number>(0)
  const finalDurationRef = useRef<number>(0)

  const clearTimer = useCallback(() => {
    if (timerRef.current !== null) {
      window.clearInterval(timerRef.current)
      timerRef.current = null
    }
  }, [])

  const releaseStream = useCallback(() => {
    if (streamRef.current) {
      streamRef.current.getTracks().forEach((t) => t.stop())
      streamRef.current = null
    }
  }, [])

  const start = useCallback(async () => {
    setError('')
    setRecording(null)
    setElapsed(0)
    if (!navigator.mediaDevices?.getUserMedia) {
      setError('此浏览器不支持录音（需要 HTTPS 环境）')
      setState('error')
      return
    }
    setState('requesting')
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true })
      streamRef.current = stream
      const mimeType = pickMimeType()
      const rec = mimeType ? new MediaRecorder(stream, { mimeType }) : new MediaRecorder(stream)
      recorderRef.current = rec
      chunksRef.current = []

      rec.ondataavailable = (e) => {
        if (e.data && e.data.size > 0) chunksRef.current.push(e.data)
      }
      rec.onstop = () => {
        clearTimer()
        const type = rec.mimeType || mimeType || 'audio/webm'
        const blob = new Blob(chunksRef.current, { type })
        const url = URL.createObjectURL(blob)
        setRecording({ blob, url, mimeType: type, durationSec: finalDurationRef.current })
        setState('recorded')
        releaseStream()
      }

      startedAtRef.current = performance.now()
      rec.start(100) // gather chunks every 100ms
      setState('recording')
      timerRef.current = window.setInterval(() => {
        const secs = (performance.now() - startedAtRef.current) / 1000
        finalDurationRef.current = secs
        setElapsed(secs)
        if (maxDurationSec > 0 && secs >= maxDurationSec) {
          clearTimer() // stop ticking before we stop the recorder
          if (rec.state !== 'inactive') rec.stop()
        }
      }, 200)
    } catch (err) {
      releaseStream()
      const e = err as DOMException
      if (e && (e.name === 'NotAllowedError' || e.name === 'SecurityError')) {
        setState('denied')
        setError('麦克风权限被拒绝')
      } else if (e && e.name === 'NotFoundError') {
        setState('error')
        setError('未检测到麦克风设备')
      } else {
        setState('error')
        setError('无法启动录音：' + (e?.message || String(err)))
      }
    }
  }, [maxDurationSec, clearTimer, releaseStream])

  const stop = useCallback(() => {
    const rec = recorderRef.current
    if (rec && rec.state !== 'inactive') {
      finalDurationRef.current = (performance.now() - startedAtRef.current) / 1000
      rec.stop()
    }
  }, [])

  const reset = useCallback(() => {
    clearTimer()
    releaseStream()
    setRecording((prev) => {
      if (prev) URL.revokeObjectURL(prev.url)
      return null
    })
    setElapsed(0)
    setError('')
    setState('idle')
  }, [clearTimer, releaseStream])

  // Cleanup on unmount.
  useEffect(() => {
    return () => {
      clearTimer()
      releaseStream()
      setRecording((prev) => {
        if (prev) URL.revokeObjectURL(prev.url)
        return null
      })
    }
  }, [clearTimer, releaseStream])

  return { state, elapsed, recording, error, start, stop, reset }
}
