import { useEffect, useState } from 'react'
import { RefreshCw } from 'lucide-react'
import { cn } from '@/lib/utils'
import { forceReload, useUpdateAvailable } from '@/lib/client-cache'

// Non-blocking banner shown when the server reports a newer build than the one
// this page is running. It never forces a reload on its own — the user clicks
// to refresh, which runs from a user gesture (reliable navigation, and it
// won't interrupt a recording in progress on /r/:key). See lib/client-cache.ts
// for the full state machine.

const RELOAD_RESET_MS = 6000

export function UpdateBanner() {
  const { available } = useUpdateAvailable()
  const [reloading, setReloading] = useState(false)

  // forceReload unloads the page on success. If it somehow doesn't, reset the
  // visual state after a few seconds so the user can click again.
  useEffect(() => {
    if (!reloading) return
    const t = window.setTimeout(() => setReloading(false), RELOAD_RESET_MS)
    return () => window.clearTimeout(t)
  }, [reloading])

  if (!available) return null

  function onClick() {
    if (reloading) return
    setReloading(true)
    void forceReload()
  }

  return (
    <div className="pointer-events-none fixed inset-x-0 top-0 z-[90] flex justify-center px-4 pt-3">
      <button
        type="button"
        onClick={onClick}
        className="pointer-events-auto flex w-full max-w-md items-center justify-center gap-2 rounded-lg border border-amber-500/40 bg-amber-500/15 px-4 py-2.5 text-center text-sm font-medium text-amber-200 shadow-lg shadow-black/20 backdrop-blur transition-colors hover:bg-amber-500/25"
      >
        <RefreshCw className={cn('h-4 w-4 shrink-0', reloading && 'animate-spin')} />
        <span>
          {reloading
            ? '正在刷新…如长时间未刷新，请再次点击此处'
            : '站点已更新，为确保您的正常使用，请点击此处刷新'}
        </span>
      </button>
    </div>
  )
}
