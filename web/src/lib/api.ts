// Typed client for the FireVoiceBox HTTP API.

export interface PublicProject {
  id: string
  slug: string
  title: string
  description: string
  max_duration_sec: number
  status: string
  accepting: boolean
}

export interface Project {
  id: string
  slug: string
  title: string
  description: string
  max_duration_sec: number
  max_per_ip: number
  status: string
  manage_token: string
  created_at: string
  submission_count: number
}

export interface Submission {
  id: string
  project_id: string
  student_id: string
  nickname: string
  ip: string
  user_agent: string
  duration_sec: number
  mime_type: string
  size_bytes: number
  created_at: string
  audio_path: string
}

export interface VersionInfo {
  version: string
  commit: string
  build_time: string
  update_channel: string
  update_repo: string
}

export interface UpdateStatus {
  state: string
  current_version: string
  latest_version?: string
  is_prerelease?: boolean
  progress?: number
  download_progress?: number
  error?: string
  last_check?: string
  release_notes?: string
}

export interface UpdateCheckResult {
  has_update: boolean
  current_version: string
  latest_version?: string
  is_prerelease?: boolean
  release_notes?: string
  channel: string
}

export interface ClientVersion {
  ok: boolean
  cache_key: string
  version: string
}

export interface AppSettings {
  server: {
    addr: string
    data_dir: string
    trusted_proxy: boolean
    max_upload_mb: number
    secret: string
  }
  admin: {
    password: string
  }
  transcode: {
    enabled: boolean
    ffmpeg_path: string
    format: string
    bitrate: string
    on_error: 'keep_original' | 'reject' | string
  }
  update: {
    enabled: boolean
    channel: 'stable' | 'dev' | string
    check_interval: number
    tag: string
    repo: string
  }
}

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

async function request<T>(path: string, opts: RequestInit = {}): Promise<T> {
  const res = await fetch(path, {
    credentials: 'same-origin',
    cache: 'no-store',
    ...opts,
  })
  if (!res.ok) {
    let msg = `request failed (${res.status})`
    try {
      const body = await res.json()
      if (body && typeof body.error === 'string') msg = body.error
    } catch {
      // ignore non-JSON error bodies
    }
    throw new ApiError(res.status, msg)
  }
  if (res.status === 204) return undefined as T
  const ct = res.headers.get('Content-Type') || ''
  if (ct.includes('application/json')) return res.json() as Promise<T>
  return undefined as T
}

function tokenHeaders(token?: string): HeadersInit {
  return token ? { 'X-Manage-Token': token } : {}
}

function cacheBusted(path: string): string {
  const sep = path.includes('?') ? '&' : '?'
  return `${path}${sep}_=${Date.now()}`
}

export const api = {
  // Client cache coordination
  clientVersion: (fresh = false) =>
    request<ClientVersion>(fresh ? cacheBusted(`/api/client/version`) : `/api/client/version`),

  // Public recording page
  getPublicProject: (key: string) => request<PublicProject>(`/api/p/${encodeURIComponent(key)}`),
  submit: (key: string, form: FormData) =>
    request<{ id: string }>(`/api/p/${encodeURIComponent(key)}/submissions`, {
      method: 'POST',
      body: form,
    }),

  // Owner auth
  login: (password: string) =>
    request<{ ok: boolean }>(`/api/admin/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ password }),
    }),
  logout: () => request<{ ok: boolean }>(`/api/admin/logout`, { method: 'POST' }),
  me: () => request<{ owner: boolean }>(`/api/admin/me`),

  // Owner project management
  listProjects: () => request<Project[]>(`/api/admin/projects`),
  createProject: (data: Partial<Project>) =>
    request<Project>(`/api/admin/projects`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(data),
    }),
  updateProject: (id: string, data: Partial<Project>) =>
    request<Project>(`/api/admin/projects/${id}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(data),
    }),
  deleteProject: (id: string) =>
    request<{ ok: boolean }>(`/api/admin/projects/${id}`, { method: 'DELETE' }),
  settings: () => request<{ ok: boolean; settings: AppSettings }>(`/api/admin/settings`),
  updateSettings: (settings: AppSettings) =>
    request<{ ok: boolean; settings: AppSettings }>(`/api/admin/settings`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(settings),
    }),
  version: (fresh = false) =>
    request<{ ok: boolean; version: VersionInfo }>(fresh ? cacheBusted(`/api/admin/version`) : `/api/admin/version`),
  updateStatus: () => request<{ ok: boolean; status: UpdateStatus }>(`/api/admin/update/status`),
  checkUpdate: () =>
    request<{ ok: boolean; result: UpdateCheckResult; error?: string }>(`/api/admin/update/check`, {
      method: 'POST',
    }),
  applyUpdate: () =>
    request<{ ok?: boolean; status: string }>(`/api/admin/update/apply`, {
      method: 'POST',
    }),
  dismissUpdate: () =>
    request<{ ok: boolean }>(`/api/admin/update/dismiss`, {
      method: 'POST',
    }),

  // Project management (owner cookie OR manage token)
  getManageProject: (id: string, token?: string) =>
    request<Project>(`/api/manage/projects/${id}`, { headers: tokenHeaders(token) }),
  listSubmissions: (id: string, token?: string) =>
    request<Submission[]>(`/api/manage/projects/${id}/submissions`, { headers: tokenHeaders(token) }),
  deleteSubmission: (id: string, token?: string) =>
    request<{ ok: boolean }>(`/api/manage/submissions/${id}`, {
      method: 'DELETE',
      headers: tokenHeaders(token),
    }),

  // URLs for direct browser navigation / media elements
  audioUrl: (audioPath: string, token?: string) =>
    token ? `${audioPath}?key=${encodeURIComponent(token)}` : audioPath,
  exportUrl: (id: string, token?: string) =>
    token ? `/api/manage/projects/${id}/export?key=${encodeURIComponent(token)}` : `/api/manage/projects/${id}/export`,
}
