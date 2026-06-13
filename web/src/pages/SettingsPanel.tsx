import { useEffect, useState, type FormEvent, type ReactNode } from 'react'
import { Loader2, RefreshCw, Save, Server, Settings, Shield, Upload } from 'lucide-react'
import { api, ApiError, type AppSettings } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { useToast } from '@/components/ui/toast'

type Choice = {
  value: string
  label: string
}

export function SettingsPanel() {
  const { error, success } = useToast()
  const [settings, setSettings] = useState<AppSettings | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)

  async function load() {
    setLoading(true)
    try {
      const res = await api.settings()
      setSettings(res.settings)
    } catch (err) {
      error(err instanceof ApiError ? err.message : '加载设置失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  function patchSection<K extends keyof AppSettings>(section: K, values: Partial<AppSettings[K]>) {
    setSettings((current) =>
      current ? ({ ...current, [section]: { ...current[section], ...values } } as AppSettings) : current,
    )
  }

  async function submit(e: FormEvent) {
    e.preventDefault()
    if (!settings) return
    setSaving(true)
    try {
      const res = await api.updateSettings(settings)
      setSettings(res.settings)
      success('设置已保存')
    } catch (err) {
      error(err instanceof ApiError ? err.message : '保存设置失败')
    } finally {
      setSaving(false)
    }
  }

  if (settings === null) {
    return (
      <Card>
        <CardContent className="flex flex-col items-center gap-3 py-16 text-center">
          {loading ? (
            <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
          ) : (
            <>
              <p className="text-muted-foreground">设置加载失败。</p>
              <Button type="button" variant="outline" onClick={load}>
                <RefreshCw className="h-4 w-4" /> 重新加载
              </Button>
            </>
          )}
        </CardContent>
      </Card>
    )
  }

  return (
    <form onSubmit={submit} className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold">全局设置</h2>
          <p className="mt-1 text-sm text-muted-foreground">部分运行时设置保存后需重启。</p>
        </div>
        <div className="flex items-center gap-2">
          <Button type="button" variant="outline" onClick={load} disabled={loading || saving}>
            <RefreshCw className={loading ? 'h-4 w-4 animate-spin' : 'h-4 w-4'} /> 重新加载
          </Button>
          <Button type="submit" disabled={saving}>
            {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
            保存设置
          </Button>
        </div>
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <SettingsCard icon={<Server className="h-5 w-5" />} title="Server" description="HTTP 与存储参数。">
          <SettingsField id="server-addr" label="监听地址">
            <Input
              id="server-addr"
              value={settings.server.addr}
              onChange={(e) => patchSection('server', { addr: e.target.value })}
              placeholder=":8080"
            />
          </SettingsField>
          <SettingsField id="server-data-dir" label="数据目录">
            <Input
              id="server-data-dir"
              value={settings.server.data_dir}
              onChange={(e) => patchSection('server', { data_dir: e.target.value })}
              placeholder="./data"
            />
          </SettingsField>
          <SettingsField id="server-max-upload" label="上传上限 MB">
            <Input
              id="server-max-upload"
              type="number"
              min={1}
              value={settings.server.max_upload_mb}
              onChange={(e) => patchSection('server', { max_upload_mb: numberFromInput(e.target.value, 1) })}
            />
          </SettingsField>
          <ToggleRow
            id="server-trusted-proxy"
            label="信任反向代理"
            checked={settings.server.trusted_proxy}
            onCheckedChange={(checked) => patchSection('server', { trusted_proxy: checked })}
          />
        </SettingsCard>

        <SettingsCard icon={<Shield className="h-5 w-5" />} title="Admin" description="登录密码与会话签名。">
          <SettingsField id="admin-password" label="管理员密码">
            <Input
              id="admin-password"
              type="password"
              autoComplete="new-password"
              value={settings.admin.password}
              onChange={(e) => patchSection('admin', { password: e.target.value })}
            />
          </SettingsField>
          <SettingsField id="server-secret" label="会话签名密钥">
            <Input
              id="server-secret"
              type="password"
              autoComplete="off"
              value={settings.server.secret}
              onChange={(e) => patchSection('server', { secret: e.target.value })}
            />
          </SettingsField>
        </SettingsCard>

        <SettingsCard icon={<Upload className="h-5 w-5" />} title="Transcode" description="后续上传的音频转码。">
          <ToggleRow
            id="transcode-enabled"
            label="启用转码"
            checked={settings.transcode.enabled}
            onCheckedChange={(checked) => patchSection('transcode', { enabled: checked })}
          />
          <SettingsField id="transcode-ffmpeg" label="ffmpeg 路径">
            <Input
              id="transcode-ffmpeg"
              value={settings.transcode.ffmpeg_path}
              onChange={(e) => patchSection('transcode', { ffmpeg_path: e.target.value })}
              placeholder="ffmpeg"
            />
          </SettingsField>
          <div className="grid gap-3 sm:grid-cols-2">
            <SettingsField id="transcode-format" label="格式">
              <Input
                id="transcode-format"
                value={settings.transcode.format}
                onChange={(e) => patchSection('transcode', { format: e.target.value })}
                placeholder="mp3"
              />
            </SettingsField>
            <SettingsField id="transcode-bitrate" label="码率">
              <Input
                id="transcode-bitrate"
                value={settings.transcode.bitrate}
                onChange={(e) => patchSection('transcode', { bitrate: e.target.value })}
                placeholder="128k"
              />
            </SettingsField>
          </div>
          <SettingsField label="失败处理">
            <Segmented
              value={settings.transcode.on_error}
              options={[
                { value: 'keep_original', label: '保留原文件' },
                { value: 'reject', label: '拒绝上传' },
              ]}
              onChange={(value) => patchSection('transcode', { on_error: value })}
            />
          </SettingsField>
        </SettingsCard>

        <SettingsCard icon={<Settings className="h-5 w-5" />} title="Update" description="自动更新检查。">
          <ToggleRow
            id="update-enabled"
            label="启用自动更新"
            checked={settings.update.enabled}
            onCheckedChange={(checked) => patchSection('update', { enabled: checked })}
          />
          <SettingsField label="通道">
            <Segmented
              value={settings.update.channel}
              options={[
                { value: 'stable', label: 'Stable' },
                { value: 'dev', label: 'Dev' },
              ]}
              onChange={(value) => patchSection('update', { channel: value })}
            />
          </SettingsField>
          <SettingsField id="update-interval" label="检查间隔秒">
            <Input
              id="update-interval"
              type="number"
              min={1}
              value={settings.update.check_interval}
              onChange={(e) => patchSection('update', { check_interval: numberFromInput(e.target.value, 1) })}
            />
          </SettingsField>
          <SettingsField id="update-tag" label="固定版本标签">
            <Input
              id="update-tag"
              value={settings.update.tag}
              onChange={(e) => patchSection('update', { tag: e.target.value })}
              placeholder="留空使用最新版本"
            />
          </SettingsField>
          <SettingsField id="update-repo" label="GitHub 仓库">
            <Input
              id="update-repo"
              value={settings.update.repo}
              onChange={(e) => patchSection('update', { repo: e.target.value })}
              placeholder="lieyanc/FireVoiceBox"
            />
          </SettingsField>
        </SettingsCard>
      </div>
    </form>
  )
}

function SettingsCard({
  icon,
  title,
  description,
  children,
}: {
  icon: ReactNode
  title: string
  description: string
  children: ReactNode
}) {
  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-2">
          <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-secondary">{icon}</div>
          <div>
            <CardTitle className="text-base">{title}</CardTitle>
            <CardDescription>{description}</CardDescription>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">{children}</CardContent>
    </Card>
  )
}

function SettingsField({ id, label, children }: { id?: string; label: string; children: ReactNode }) {
  return (
    <div className="space-y-2">
      <Label htmlFor={id}>{label}</Label>
      {children}
    </div>
  )
}

function ToggleRow({
  id,
  label,
  checked,
  onCheckedChange,
}: {
  id: string
  label: string
  checked: boolean
  onCheckedChange: (checked: boolean) => void
}) {
  return (
    <div className="flex items-center justify-between gap-4 rounded-md border border-border/70 px-3 py-2.5">
      <Label htmlFor={id}>{label}</Label>
      <Switch id={id} checked={checked} onCheckedChange={onCheckedChange} />
    </div>
  )
}

function Segmented({ value, options, onChange }: { value: string; options: Choice[]; onChange: (value: string) => void }) {
  return (
    <div className="flex flex-wrap gap-1 rounded-md border border-border p-1">
      {options.map((option) => (
        <Button
          key={option.value}
          type="button"
          size="sm"
          variant={value === option.value ? 'secondary' : 'ghost'}
          onClick={() => onChange(option.value)}
        >
          {option.label}
        </Button>
      ))}
    </div>
  )
}

function numberFromInput(value: string, min: number) {
  const parsed = Number(value)
  if (!Number.isFinite(parsed)) return min
  return Math.max(min, Math.trunc(parsed))
}
