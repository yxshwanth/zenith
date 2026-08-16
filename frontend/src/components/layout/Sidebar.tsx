import { Link, useLocation } from 'react-router-dom'
import {
  ChartBarIcon,
  CheckCircleIcon,
  CubeIcon,
  TableCellsIcon,
  ClockIcon,
  XMarkIcon,
} from '@heroicons/react/24/outline'
import clsx from 'clsx'

interface SidebarProps {
  open: boolean
  onClose: () => void
}

const navigation = [
  { name: 'Graph Visualizer', href: '/', icon: ChartBarIcon },
  { name: 'Permission Checker', href: '/check', icon: CheckCircleIcon },
  { name: 'Batch Checker', href: '/batch', icon: CubeIcon },
  { name: 'Tuple Manager', href: '/tuples', icon: TableCellsIcon },
  { name: 'Performance Dashboard', href: '/dashboard', icon: ChartBarIcon },
  { name: 'Audit Trail', href: '/audit', icon: ClockIcon },
]

export default function Sidebar({ open, onClose }: SidebarProps) {
  const location = useLocation()

  return (
    <>
      {/* Mobile overlay */}
      {open && (
        <div
          className="fixed inset-0 bg-gray-600 bg-opacity-75 z-40 lg:hidden"
          onClick={onClose}
        />
      )}

      {/* Sidebar */}
      <aside
        className={clsx(
          'fixed top-16 left-0 z-40 h-[calc(100vh-4rem)] w-64 bg-white dark:bg-gray-800 border-r border-gray-200 dark:border-gray-700 transform transition-transform duration-300 ease-in-out lg:translate-x-0',
          open ? 'translate-x-0' : '-translate-x-full'
        )}
      >
        <div className="flex flex-col h-full">
          <div className="flex items-center justify-between p-4 lg:hidden">
            <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">
              Menu
            </h2>
            <button
              onClick={onClose}
              className="p-2 rounded-md text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700"
            >
              <XMarkIcon className="h-6 w-6" />
            </button>
          </div>

          <nav className="flex-1 px-4 py-4 space-y-1 overflow-y-auto">
            {navigation.map((item) => {
              const isActive = location.pathname === item.href
              return (
                <Link
                  key={item.name}
                  to={item.href}
                  onClick={onClose}
                  className={clsx(
                    'flex items-center px-4 py-3 text-sm font-medium rounded-lg transition-colors',
                    isActive
                      ? 'bg-primary-100 dark:bg-primary-900 text-primary-700 dark:text-primary-300'
                      : 'text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700'
                  )}
                >
                  <item.icon className="mr-3 h-5 w-5" />
                  {item.name}
                </Link>
              )
            })}
          </nav>
        </div>
      </aside>
    </>
  )
}

