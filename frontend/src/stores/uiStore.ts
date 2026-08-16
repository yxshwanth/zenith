import { create } from 'zustand'

interface UIState {
  darkMode: boolean
  sidebarOpen: boolean
  toggleDarkMode: () => void
  setSidebarOpen: (open: boolean) => void
}

export const useUIStore = create<UIState>((set) => ({
  darkMode: localStorage.getItem('darkMode') === 'true',
  sidebarOpen: false,
  toggleDarkMode: () =>
    set((state) => {
      const newMode = !state.darkMode
      localStorage.setItem('darkMode', String(newMode))
      document.documentElement.classList.toggle('dark', newMode)
      return { darkMode: newMode }
    }),
  setSidebarOpen: (open: boolean) => set({ sidebarOpen: open }),
}))

// Initialize dark mode on load
if (localStorage.getItem('darkMode') === 'true') {
  document.documentElement.classList.add('dark')
}

