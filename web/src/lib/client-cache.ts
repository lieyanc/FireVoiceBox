import { useEffect } from 'react'
import { api } from '@/lib/api'

const clientCacheKeyStorageKey = 'firevoicebox.clientCacheKey'
const clientCachePollIntervalMs = 60_000

let reloading = false

function readStoredClientCacheKey() {
  try {
    return window.localStorage.getItem(clientCacheKeyStorageKey)
  } catch {
    return null
  }
}

function writeStoredClientCacheKey(key: string) {
  try {
    window.localStorage.setItem(clientCacheKeyStorageKey, key)
  } catch {
    // Storage can be unavailable in locked-down browsers.
  }
}

async function clearClientCaches() {
  if (!('caches' in window)) return
  const names = await window.caches.keys()
  await Promise.all(names.map((name) => window.caches.delete(name)))
}

function reloadWithFreshEntry(key: string) {
  const url = new URL(window.location.href)
  url.searchParams.set('fvb_refresh', key || String(Date.now()))
  window.location.replace(url.toString())
}

export async function refreshIfClientCacheChanged(cacheKey: string, onBeforeReload?: () => void) {
  if (!cacheKey || reloading) return false

  const knownKey = readStoredClientCacheKey()
  if (knownKey && knownKey !== cacheKey) {
    reloading = true
    writeStoredClientCacheKey(cacheKey)
    onBeforeReload?.()
    await clearClientCaches().catch(() => undefined)
    reloadWithFreshEntry(cacheKey)
    return true
  }

  writeStoredClientCacheKey(cacheKey)
  return false
}

export function useClientCacheRefresh() {
  useEffect(() => {
    let stopped = false

    async function check() {
      try {
        const data = await api.clientVersion(true)
        if (!stopped) {
          await refreshIfClientCacheChanged(data.cache_key)
        }
      } catch {
        // The server may be restarting during an update; the next poll will retry.
      }
    }

    void check()
    const timer = window.setInterval(check, clientCachePollIntervalMs)
    return () => {
      stopped = true
      window.clearInterval(timer)
    }
  }, [])
}
