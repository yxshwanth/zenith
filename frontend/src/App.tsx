import { BrowserRouter, Routes, Route } from 'react-router-dom'
import { useState, Suspense } from 'react'
import { ErrorBoundary } from './components/layout/ErrorBoundary'
import Navbar from './components/layout/Navbar'
import Sidebar from './components/layout/Sidebar'
import PermissionGraph from './components/graph/PermissionGraph'
import PermissionChecker from './components/checker/PermissionChecker'
import BatchChecker from './components/checker/BatchChecker'
import TupleManager from './components/management/TupleManager'
import PerformanceDashboard from './components/dashboard/PerformanceDashboard'
import TemporalViewer from './components/audit/TemporalViewer'

function LoadingFallback() {
  return (
    <div className="card">
      <div className="flex items-center justify-center h-96">
        <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-primary-600"></div>
      </div>
    </div>
  )
}

function App() {
  const [sidebarOpen, setSidebarOpen] = useState(false)

  return (
    <ErrorBoundary>
      <BrowserRouter>
        <div className="min-h-screen bg-gray-50 dark:bg-gray-900">
          <Navbar onMenuClick={() => setSidebarOpen(!sidebarOpen)} />
          <div className="flex pt-16">
            <Sidebar open={sidebarOpen} onClose={() => setSidebarOpen(false)} />
            <main className="flex-1 p-4 md:p-6 lg:ml-64">
              <Suspense fallback={<LoadingFallback />}>
                <Routes>
                  <Route path="/" element={<PermissionGraph />} />
                  <Route path="/check" element={<PermissionChecker />} />
                  <Route path="/batch" element={<BatchChecker />} />
                  <Route path="/tuples" element={<TupleManager />} />
                  <Route path="/dashboard" element={<PerformanceDashboard />} />
                  <Route path="/audit" element={<TemporalViewer />} />
                </Routes>
              </Suspense>
            </main>
          </div>
        </div>
      </BrowserRouter>
    </ErrorBoundary>
  )
}

export default App

