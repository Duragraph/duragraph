import type { Message } from '@/types/entities'
import ReactMarkdown from 'react-markdown'

interface ChatMessageProps {
  message: Message
  isStreaming?: boolean
}

export function ChatMessage({ message, isStreaming }: ChatMessageProps) {
  const isUser = message.role === 'user'

  return (
    <div className={`flex ${isUser ? 'justify-end' : 'justify-start'}`}>
      <div
        className={`max-w-[80%] px-4 py-3 text-sm ${
          isUser
            ? 'bg-primary text-primary-foreground'
            : 'bg-card shadow-card'
        }`}
      >
        {isUser ? (
          <p className="whitespace-pre-wrap">{message.content}</p>
        ) : (
          <div className="prose prose-sm max-w-none dark:prose-invert prose-p:my-1 prose-pre:my-2">
            <ReactMarkdown>{message.content}</ReactMarkdown>
          </div>
        )}
        {isStreaming && (
          <span className="ml-1 inline-block h-3 w-1.5 animate-pulse bg-foreground/60" />
        )}
      </div>
    </div>
  )
}
