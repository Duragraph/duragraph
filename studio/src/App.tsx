import { useUIStore } from '@/stores/ui'
import { Header } from '@/components/layout/Header'
import { Sidebar } from '@/components/layout/Sidebar'
import { ChatView } from '@/views/ChatView'
import { TracesView } from '@/views/TracesView'

function App() {
  const { activeView } = useUIStore()

  return (
    <div className="flex h-screen flex-col bg-background text-foreground">
      <Header />
      <div className="flex flex-1 overflow-hidden">
        <Sidebar />
        <main className="flex-1 overflow-hidden">
          {activeView === 'chat' && <ChatView />}
          {activeView === 'traces' && <TracesView />}
        </main>
      </div>
    </div>
  )
}

export default App
