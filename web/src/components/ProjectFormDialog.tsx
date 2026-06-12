import { useEffect, useState } from 'react'
import { Loader2 } from 'lucide-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Switch } from '@/components/ui/switch'
import type { Project } from '@/lib/api'

export interface ProjectFormValues {
  title: string
  description: string
  slug: string
  max_duration_sec: number
  max_per_ip: number
  status: string
}

interface ProjectFormDialogProps {
  mode: 'create' | 'edit'
  open: boolean
  onOpenChange: (open: boolean) => void
  initial?: Project
  onSubmit: (values: Partial<Project>) => Promise<void>
}

export function ProjectFormDialog({ mode, open, onOpenChange, initial, onSubmit }: ProjectFormDialogProps) {
  const [values, setValues] = useState<ProjectFormValues>({
    title: '',
    description: '',
    slug: '',
    max_duration_sec: 60,
    max_per_ip: 1,
    status: 'open',
  })
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (open) {
      setValues({
        title: initial?.title ?? '',
        description: initial?.description ?? '',
        slug: initial?.slug ?? '',
        max_duration_sec: initial?.max_duration_sec ?? 60,
        max_per_ip: initial?.max_per_ip ?? 1,
        status: initial?.status ?? 'open',
      })
    }
  }, [open, initial])

  function set<K extends keyof ProjectFormValues>(k: K, v: ProjectFormValues[K]) {
    setValues((prev) => ({ ...prev, [k]: v }))
  }

  async function handleSubmit() {
    setSaving(true)
    try {
      await onSubmit({
        title: values.title.trim(),
        description: values.description,
        slug: values.slug.trim(),
        max_duration_sec: values.max_duration_sec,
        max_per_ip: values.max_per_ip,
        status: values.status,
      })
      onOpenChange(false)
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{mode === 'create' ? '新建项目' : '编辑项目'}</DialogTitle>
          <DialogDescription>设置标题、描述与上传限制。</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="f-title">标题</Label>
            <Input id="f-title" value={values.title} onChange={(e) => set('title', e.target.value)} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="f-desc">描述</Label>
            <Textarea id="f-desc" value={values.description} onChange={(e) => set('description', e.target.value)} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="f-slug">短链接 slug（可选）</Label>
            <Input
              id="f-slug"
              value={values.slug}
              onChange={(e) => set('slug', e.target.value)}
              placeholder="留空则自动生成"
            />
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="f-dur">单条最长（秒）</Label>
              <Input
                id="f-dur"
                type="number"
                min={1}
                value={values.max_duration_sec}
                onChange={(e) => set('max_duration_sec', Math.max(1, Number(e.target.value) || 0))}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="f-ip">单 IP 上传数（0=不限）</Label>
              <Input
                id="f-ip"
                type="number"
                min={0}
                value={values.max_per_ip}
                onChange={(e) => set('max_per_ip', Math.max(0, Number(e.target.value) || 0))}
              />
            </div>
          </div>
          <div className="flex items-center justify-between rounded-lg border p-3">
            <div>
              <Label htmlFor="f-status">开放收集</Label>
              <p className="text-xs text-muted-foreground">关闭后用户将无法上传</p>
            </div>
            <Switch
              id="f-status"
              checked={values.status === 'open'}
              onCheckedChange={(c) => set('status', c ? 'open' : 'closed')}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
            取消
          </Button>
          <Button onClick={handleSubmit} disabled={saving || !values.title.trim()}>
            {saving && <Loader2 className="h-4 w-4 animate-spin" />}
            {mode === 'create' ? '创建' : '保存'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
