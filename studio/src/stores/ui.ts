import { create } from 'zustand'

type View = 'chat' | 'traces'

interface UIState {
  activeView: View
  sidebarOpen: boolean
  setView: (view: View) => void
  toggleSidebar: () => void
}

export const useUIStore = create<UIState>((set) => ({
  activeView: 'chat',
  sidebarOpen: true,
  setView: (view) => set({ activeView: view }),
  toggleSidebar: () => set((s) => ({ sidebarOpen: !s.sidebarOpen })),
}))
