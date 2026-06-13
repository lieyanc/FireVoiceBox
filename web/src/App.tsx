import { Navigate, Route, Routes } from 'react-router-dom'
import { RecordPage } from '@/pages/RecordPage'
import { AdminHome } from '@/pages/AdminHome'
import { ManageView } from '@/pages/ManageView'
import { useClientCacheRefresh } from '@/lib/client-cache'

export default function App() {
  useClientCacheRefresh()

  return (
    <Routes>
      <Route path="/" element={<Navigate to="/admin" replace />} />
      <Route path="/r/:key" element={<RecordPage />} />
      <Route path="/admin" element={<AdminHome />} />
      {/* Owner-authenticated project detail (uses session cookie). */}
      <Route path="/admin/projects/:id" element={<ManageView />} />
      {/* Token-shared project view (manage token comes from the URL hash). */}
      <Route path="/admin/p/:id" element={<ManageView fromHash />} />
      <Route path="*" element={<NotFound />} />
    </Routes>
  )
}

function NotFound() {
  return (
    <div className="flex min-h-screen items-center justify-center p-6 text-center">
      <div>
        <h1 className="text-2xl font-semibold">404</h1>
        <p className="mt-2 text-muted-foreground">页面不存在</p>
      </div>
    </div>
  )
}
