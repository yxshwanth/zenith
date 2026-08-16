import { Link } from 'react-router-dom'
import { useUIStore } from '../../stores/uiStore'
import { MoonIcon, SunIcon, Bars3Icon } from '@heroicons/react/24/outline'

interface NavbarProps {
  onMenuClick: () => void
}

export default function Navbar({ onMenuClick }: NavbarProps) {
  const { darkMode, toggleDarkMode } = useUIStore()

  return (
    <nav className="bg-white dark:bg-gray-800 shadow-sm border-b border-gray-200 dark:border-gray-700 fixed w-full top-0 z-50">
      <div className="px-4 sm:px-6 lg:px-8">
        <div className="flex justify-between h-16">
          <div className="flex items-center">
            <button
              onClick={onMenuClick}
              className="lg:hidden p-2 rounded-md text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700"
            >
              <Bars3Icon className="h-6 w-6" />
            </button>
            <Link to="/" className="flex items-center ml-4 lg:ml-0">
              <span className="text-2xl font-bold text-primary-600 dark:text-primary-400">
                Zenith
              </span>
              <span className="ml-2 text-sm text-gray-500 dark:text-gray-400">
                Permission System
              </span>
            </Link>
          </div>
          <div className="flex items-center space-x-4">
            <button
              onClick={toggleDarkMode}
              className="p-2 rounded-md text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700"
              aria-label="Toggle dark mode"
            >
              {darkMode ? (
                <SunIcon className="h-6 w-6" />
              ) : (
                <MoonIcon className="h-6 w-6" />
              )}
            </button>
          </div>
        </div>
      </div>
    </nav>
  )
}

