import { useUIStore } from '@/stores/ui'

export function Header() {
  const { activeView, setView } = useUIStore()

  const navItems = [
    { key: 'chat' as const, label: 'Chat' },
    { key: 'traces' as const, label: 'Traces' },
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
    </header>
  )
}
