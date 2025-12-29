import { useState } from 'react'

function App() {
  const [message, setMessage] = useState('')

  return (
    <div className="min-h-screen bg-background text-foreground">
      {/* Header */}
      <header className="border-b border-border px-6 py-4">
        <div className="flex items-center justify-between">
          <h1 className="text-xl font-semibold">DuraGraph Studio</h1>
          <nav className="flex gap-4">
            <button className="text-sm text-muted-foreground hover:text-foreground">
              Chat
            </button>
            <button className="text-sm text-muted-foreground hover:text-foreground">
              Traces
            </button>
            <button className="text-sm text-muted-foreground hover:text-foreground">
              Approvals
            </button>
          </nav>
        </div>
      </header>

      {/* Main Content */}
      <main className="flex h-[calc(100vh-65px)]">
        {/* Sidebar */}
        <aside className="w-64 border-r border-border p-4">
          <div className="mb-4">
            <label className="text-sm font-medium">Thread</label>
            <select className="mt-1 w-full rounded-md border border-input bg-background px-3 py-2 text-sm">
              <option>New Thread</option>
            </select>
          </div>
          <div>
            <label className="text-sm font-medium">Assistant</label>
            <select className="mt-1 w-full rounded-md border border-input bg-background px-3 py-2 text-sm">
              <option>Select Assistant</option>
            </select>
          </div>
        </aside>

        {/* Chat Area */}
        <div className="flex flex-1 flex-col">
          {/* Messages */}
          <div className="flex-1 overflow-y-auto p-6">
            <div className="mx-auto max-w-3xl">
              <div className="text-center text-muted-foreground">
                <p className="text-lg">Welcome to DuraGraph Studio</p>
                <p className="mt-2 text-sm">
                  Select an assistant and start a conversation
                </p>
              </div>
            </div>
          </div>

          {/* Input */}
          <div className="border-t border-border p-4">
            <div className="mx-auto max-w-3xl">
              <div className="flex gap-2">
                <input
                  type="text"
                  value={message}
                  onChange={(e) => setMessage(e.target.value)}
                  placeholder="Type your message..."
                  className="flex-1 rounded-md border border-input bg-background px-4 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-ring"
                />
                <button className="rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90">
                  Send
                </button>
              </div>
            </div>
          </div>
        </div>
      </main>
    </div>
  )
}

export default App
