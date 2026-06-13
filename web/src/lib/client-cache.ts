import { useEffect, useSyncExternalStore } from 'react'
import { api } from '@/lib/api'

// Client cache coordination.
//
// The server exposes its build identity as a short cache key
// (version.ClientCacheKey in the Go code). This module polls it and, when it
// changes, surfaces a "site updated — please refresh" prompt rather than
// forcing an automatic reload. The reload only fires from a user gesture
// (forceReload, via the UpdateBanner button) so that:
//   - the navigation is reliable. Background tabs and mobile WebViews often
//     throttle or silently drop non-user-activated navigations — the previous
//     auto-reload failed "some of the time" for exactly this reason.
//   - it never interrupts a user mid-recording or mid-form-entry.
//
// Why the baseline key lives in memory (not localStorage): each browser tab
// must judge "the build I'm running" independently. With localStorage,
// refreshing tab A to a new build would write the new key, causing a stale
// tab B to read it on its next poll and wrongly conclude it is already up to
// date — silently pinned to the old JS forever. An in-module variable resets
// on every page load, so each tab gets its own correct baseline. This works
// because index.html is served no-store, so the key observed on first poll
// after a load always matches the JS bundle that page is actually running.
//
// State machine (what makes the prompt self-correcting):
//   - On the first poll after a page load, loadedKey is set to the server's
//     current key. That is the baseline for this tab — never an "update".
//   - On later polls, if the server's key differs from loadedKey, we mark an
//     update available and show the banner. We do NOT change loadedKey.
//   - The user clicks refresh -> forceReload navigates with ?fvb_refresh
//     (origin emits Clear-Site-Data:"cache"). The freshly loaded page sees
//     fvb_refresh, treats that first poll as a post-refresh landing, strips
//     the param, and uses the new key as its baseline.
//   - If the navigation never lands, loadedKey is unchanged, the banner stays
//     up, and the user can click again. No silent failure is possible.

const clientCachePollIntervalMs = 60_000
const forceRefreshQueryParam = 'fvb_refresh'

// --- external store backing useUpdateAvailable ---

export interface ClientUpdateState {
  available: boolean
  fromKey: string | null
  toKey: string | null
}

const idleState: ClientUpdateState = { available: false, fromKey: null, toKey: null }

let updateState: ClientUpdateState = idleState
const listeners = new Set<() => void>()

function emit() {
  for (const l of listeners) l()
}

function markUpdateAvailable(fromKey: string, toKey: string) {
  // Same update already surfaced — avoid re-emitting and re-rendering callers
  // (the poller and UpdatePanel can both report the same key repeatedly).
  if (updateState.available && updateState.toKey === toKey) return
  updateState = { available: true, fromKey, toKey }
  emit()
}

function subscribe(callback: () => void) {
  listeners.add(callback)
  return () => {
    listeners.delete(callback)
  }
}

function getSnapshot() {
  return updateState
}

// --- defensive cache cleanup (no-op unless a service worker is registered) ---

async function clearClientCaches() {
  if (!('caches' in window)) return
  const names = await window.caches.keys()
  await Promise.all(names.map((name) => window.caches.delete(name)))
}

// --- the reconciliation core, shared by the poller and explicit checks ---

// The cache key this tab loaded with. null until the first successful poll,
// which establishes the baseline. In-module so each tab is independent.
let loadedKey: string | null = null

function applyCacheKey(cacheKey: string) {
  if (!cacheKey) return

  const url = new URL(window.location.href)
  const landedFromRefresh = url.searchParams.has(forceRefreshQueryParam)

  if (loadedKey === null) {
    // First observation for this page load. Because index.html is no-store,
    // the server's current key here is exactly the build this JS bundle
    // belongs to — record it as this tab's baseline. If we arrived via a
    // forceReload, strip the one-shot param so it doesn't linger in shared
    // or bookmarked URLs.
    loadedKey = cacheKey
    if (landedFromRefresh) {
      url.searchParams.delete(forceRefreshQueryParam)
      window.history.replaceState(null, '', url.toString())
    }
    return
  }

  if (loadedKey !== cacheKey) {
    // The server moved to a newer build than this tab is running. Prompt;
    // leave loadedKey alone so the banner persists and retries stay effective.
    markUpdateAvailable(loadedKey, cacheKey)
  }
}

// --- public API ---

/** Subscribe to the global "a newer build is available" state. */
export function useUpdateAvailable(): ClientUpdateState {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
}

/** Feed a freshly-fetched cache key into the coordinator (e.g. from UpdatePanel). */
export function noteClientCacheKey(cacheKey: string) {
  applyCacheKey(cacheKey)
}

// forceReload guard. Reset on a timer so a blocked navigation still lets the
// user retry from the banner (normally the page unloads first and this never
// fires).
let reloading = false
let reloadingTimer: number | undefined

/**
 * Hard-refresh the page: clears client caches and navigates with ?fvb_refresh
 * (which makes the origin emit Clear-Site-Data). Runs from a user gesture.
 * Safe to call repeatedly — a short guard prevents concurrent navigations and
 * self-resets if the navigation doesn't land.
 */
export async function forceReload() {
  if (reloading) return
  reloading = true
  window.clearTimeout(reloadingTimer)
  reloadingTimer = window.setTimeout(() => {
    reloading = false
  }, 3000)

  const toKey = updateState.toKey || ''
  await clearClientCaches().catch(() => undefined)
  const url = new URL(window.location.href)
  // The origin only checks for the param's presence (Query().Has), so we suffix
  // a timestamp to make every click a distinct URL and fully bypass any cache.
  url.searchParams.set(
    forceRefreshQueryParam,
    toKey ? `${toKey}.${Date.now()}` : String(Date.now()),
  )
  window.location.replace(url.toString())
}

/** Hook: poll /api/client/version for cache-key changes for the lifetime of the app. */
export function useClientCacheRefresh() {
  useEffect(() => {
    let stopped = false
    async function check() {
      try {
        const data = await api.clientVersion(true)
        if (!stopped) applyCacheKey(data.cache_key)
      } catch {
        // The server may be mid-restart during an OTA apply; retry next tick.
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
