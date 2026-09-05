import { Route, Routes } from 'react-router-dom'
import { AuthProvider } from './auth/AuthContext'
import { Layout } from './components/Layout'
import { RequireAuth } from './components/RequireAuth'
import { AgentsPage } from './pages/AgentsPage'
import { BoardPage } from './pages/BoardPage'
import { IssueDetailPage } from './pages/IssueDetailPage'
import { LoginPage } from './pages/LoginPage'
import { MemberProfilePage } from './pages/MemberProfilePage'
import { NotificationsPage } from './pages/NotificationsPage'
import { ProjectDetailPage } from './pages/ProjectDetailPage'
import { ProjectsPage } from './pages/ProjectsPage'

export default function App() {
  return (
    <AuthProvider>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route element={<RequireAuth />}>
          <Route element={<Layout />}>
            <Route path="/" element={<ProjectsPage />} />
            <Route path="/projects/:id" element={<ProjectDetailPage />} />
            <Route path="/projects/:id/board" element={<BoardPage />} />
            <Route path="/projects/:id/issues/:issueId" element={<IssueDetailPage />} />
            <Route path="/notifications" element={<NotificationsPage />} />
            <Route path="/agents" element={<AgentsPage />} />
            <Route path="/members/:userId" element={<MemberProfilePage />} />
          </Route>
        </Route>
      </Routes>
    </AuthProvider>
  )
}
