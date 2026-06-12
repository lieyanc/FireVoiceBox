import * as React from 'react'
import { cn } from '@/lib/utils'

type ToastVariant = 'default' | 'success' | 'error'
interface ToastItem {
  id: number
  message: string
  variant: ToastVariant
}

interface ToastContextValue {
  toast: (message: string, variant?: ToastVariant) => void
  success: (message: string) => void
  error: (message: string) => void
}

const ToastContext = React.createContext<ToastContextValue | null>(null)

let counter = 0

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [items, setItems] = React.useState<ToastItem[]>([])

  const remove = React.useCallback((id: number) => {
    setItems((prev) => prev.filter((t) => t.id !== id))
  }, [])

  const toast = React.useCallback(
    (message: string, variant: ToastVariant = 'default') => {
      const id = ++counter
      setItems((prev) => [...prev, { id, message, variant }])
      window.setTimeout(() => remove(id), 4000)
    },
    [remove],
  )

  const value = React.useMemo<ToastContextValue>(
    () => ({
      toast,
      success: (m) => toast(m, 'success'),
      error: (m) => toast(m, 'error'),
    }),
    [toast],
  )

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div className="pointer-events-none fixed bottom-4 right-4 z-[100] flex w-full max-w-sm flex-col gap-2">
        {items.map((t) => (
          <div
            key={t.id}
            className={cn(
              'pointer-events-auto cursor-pointer rounded-md border px-4 py-3 text-sm shadow-lg transition-all',
              t.variant === 'error' && 'border-destructive/50 bg-destructive text-destructive-foreground',
              t.variant === 'success' && 'border-emerald-700 bg-emerald-600 text-white',
              t.variant === 'default' && 'bg-card text-card-foreground',
            )}
            onClick={() => remove(t.id)}
          >
            {t.message}
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  )
}

export function useToast(): ToastContextValue {
  const ctx = React.useContext(ToastContext)
  if (!ctx) throw new Error('useToast must be used within ToastProvider')
  return ctx
}
