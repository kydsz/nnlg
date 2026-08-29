import { createBrowserRouter, Navigate } from 'react-router-dom'
import { RequireAuth, RedirectIfAuthed, AdminGate } from './guards'
import { isMobileDevice } from '@/hooks/useIsMobile'
import Login from '@/views/Login'
import NotFound from '@/views/NotFound'
import AdminLayout from '@/views/admin/Layout'
import MobileLayout from '@/views/mobile/Layout'

import Dashboard from '@/views/admin/Dashboard'
import Campus from '@/views/admin/Campus'
import Colleges from '@/views/admin/Colleges'
import ResearchRooms from '@/views/admin/ResearchRooms'
import Users from '@/views/admin/Users'
import Roles from '@/views/admin/Roles'
import Dimensions from '@/views/admin/Dimensions'
import Tasks from '@/views/admin/Tasks'
import Evaluations from '@/views/admin/Evaluations'
import Stats from '@/views/admin/Stats'
import DataSync from '@/views/admin/DataSync'
import CourseSchedule from '@/views/admin/CourseSchedule'
import TeacherEvaluationSummary from '@/views/admin/TeacherEvaluationSummary'

import Home from '@/views/mobile/Home'
import Schedule from '@/views/mobile/Schedule'
import Evaluation from '@/views/mobile/Evaluation'
import Evaluated from '@/views/mobile/Evaluated'
import Profile from '@/views/mobile/Profile'

export const router = createBrowserRouter([
  {
    path: '/login',
    element: (
      <RedirectIfAuthed>
        <Login />
      </RedirectIfAuthed>
    ),
  },
  {
    path: '/admin',
    element: <RequireAuth />,
    children: [
      {
        element: <AdminGate />,
        children: [
          {
            element: <AdminLayout />,
            children: [
              { index: true, element: <Navigate to="/admin/dashboard" replace /> },
              { path: 'dashboard', element: <Dashboard /> },
              { path: 'campus', element: <Campus /> },
              { path: 'colleges', element: <Colleges /> },
              { path: 'research-rooms', element: <ResearchRooms /> },
              { path: 'users', element: <Users /> },
              { path: 'roles', element: <Roles /> },
              { path: 'dimensions', element: <Dimensions /> },
              { path: 'tasks', element: <Tasks /> },
              { path: 'evaluations', element: <Evaluations /> },
              { path: 'stats', element: <Stats /> },
              { path: 'data-sync', element: <DataSync /> },
              { path: 'course-schedule', element: <CourseSchedule /> },
              { path: 'teacher-evaluation-summary', element: <TeacherEvaluationSummary /> },
            ],
          },
        ],
      },
    ],
  },
  {
    path: '/mobile',
    element: <RequireAuth />,
    children: [
      {
        element: <MobileLayout />,
        children: [
          { index: true, element: <Navigate to="/mobile/home" replace /> },
          { path: 'home', element: <Home /> },
          { path: 'schedule', element: <Schedule /> },
          { path: 'evaluation/:id', element: <Evaluation /> },
          { path: 'evaluated', element: <Evaluated /> },
          { path: 'profile', element: <Profile /> },
        ],
      },
    ],
  },
  { path: '/', element: <RootRedirect /> },
  { path: '*', element: <NotFound /> },
])

function RootRedirect() {
  const raw = localStorage.getItem('te-auth')
  let state: { token?: string; user?: { roles?: string[] } } | null = null
  try {
    state = raw ? (JSON.parse(raw) as { state?: { token?: string; user?: { roles?: string[] } } })?.state ?? null : null
  } catch {
    state = null
  }
  if (!state?.token) return <Navigate to="/login" replace />
  const roles = state.user?.roles || []
  const isAdmin = roles.some((r) =>
    ['system_admin', 'college_admin', 'school_admin'].includes(r)
  )
  // 桌面管理员进管理端；移动端管理员默认进移动端（可在“我的”切换管理端）
  if (isAdmin && !isMobileDevice()) return <Navigate to="/admin/dashboard" replace />
  return <Navigate to="/mobile/home" replace />
}
