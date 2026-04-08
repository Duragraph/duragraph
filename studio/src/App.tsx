import { useUIStore } from '@/stores/ui'
import { Header } from '@/components/layout/Header'
import { Sidebar } from '@/components/layout/Sidebar'
import { ChatView } from '@/views/ChatView'
import { TracesView } from '@/views/TracesView'
import { EditorView } from '@/views/EditorView'
import { DeploymentsView } from '@/views/DeploymentsView'

function App() {
  const { activeView } = useUIStore()

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
