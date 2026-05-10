import { useUIStore } from '@/stores/ui'
import { useAuthStore } from '@/stores/auth'
import { Header } from '@/components/layout/Header'
import { Sidebar } from '@/components/layout/Sidebar'
import { AuthView } from '@/views/AuthView'
import { ChatView } from '@/views/ChatView'
import { TracesView } from '@/views/TracesView'
import { EditorView } from '@/views/EditorView'
import { DeploymentsView } from '@/views/DeploymentsView'

function App() {
  const { activeView } = useUIStore()
  const token = useAuthStore((s) => s.token)

  // Auth gate: when no JWT in the persisted store, render the login /
  // register view instead of the app shell. lib/api.ts clears the token
  // on 401, which causes this re-render and bounces the user back to
  // the login screen on session expiry.
  if (!token) {
    return <AuthView />
  }

  const showSidebar = activeView === 'chat' || activeView === 'traces'

  return (
    <div className="flex h-screen flex-col bg-background text-foreground">
      <Header />
      <div className="flex flex-1 overflow-hidden">
        {showSidebar && <Sidebar />}
        <main className="flex-1 overflow-hidden">
          {activeView === 'chat' && <ChatView />}
          {activeView === 'traces' && <TracesView />}
          {activeView === 'editor' && <EditorView />}
          {activeView === 'deployments' && <DeploymentsView />}
        </main>
      </div>
    </div>
  )
}

export default App
