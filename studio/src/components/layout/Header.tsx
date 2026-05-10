import { useUIStore } from '@/stores/ui'
import { useAuthStore } from '@/stores/auth'

export function Header() {
  const { activeView, setView } = useUIStore()
  const user = useAuthStore((s) => s.user)
  const clearAuth = useAuthStore((s) => s.clearAuth)

  const navItems = [
    { key: 'chat' as const, label: 'Chat' },
    { key: 'traces' as const, label: 'Traces' },
    { key: 'editor' as const, label: 'Graph Editor' },
    { key: 'deployments' as const, label: 'Deployments' },
  ]

  return (
    <header className="flex items-center justify-between border-b border-border bg-card px-6 py-3">
      <div className="flex items-center gap-3">
        <h1 className="text-lg font-semibold">
          <span className="text-primary">Dura</span>Graph Studio
        </h1>
      </div>

      <nav className="flex gap-1">
        {navItems.map(({ key, label }) => (
          <button
            key={key}
            onClick={() => setView(key)}
            className={`px-4 py-1.5 text-sm font-medium transition-colors ${
              activeView === key
                ? 'bg-primary text-primary-foreground'
                : 'text-muted-foreground hover:text-foreground hover:bg-accent'
            }`}
          >
            {label}
          </button>
        ))}
      </nav>

      <div className="flex items-center gap-3">
        {user && (
          <span className="text-xs text-muted-foreground" title={user.email}>
            {user.email}
            {user.role === 'admin' && (
              <span className="ml-1.5 rounded-sm bg-primary/10 px-1.5 py-0.5 text-[10px] font-semibold text-primary">
                ADMIN
              </span>
            )}
          </span>
        )}
        <button
          type="button"
          onClick={clearAuth}
          className="px-3 py-1.5 text-xs font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
        >
          Sign out
        </button>
      </div>
    </header>
  )
}
