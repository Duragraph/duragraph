import { User, Bot, AlertCircle } from "lucide-react"
import type { Message } from "@/types/entities"
import { cn } from "@/lib/utils"

interface ChatMessageProps {
  message: Message
  isStreaming?: boolean
}

export function ChatMessage({ message, isStreaming = false }: ChatMessageProps) {
  const isUser = message.role === "user"
  const isSystem = message.role === "system"

  return (
    <div
      className={cn(
        "flex gap-3 py-4",
        isUser ? "flex-row" : "flex-row"
      )}
    >
      <div
        className={cn(
          "flex-shrink-0 w-8 h-8 rounded-full flex items-center justify-center",
          isUser
            ? "bg-primary text-primary-foreground"
            : isSystem
              ? "bg-yellow-100 dark:bg-yellow-900/30"
              : "bg-green-100 dark:bg-green-900/30"
        )}
      >
        {isUser ? (
          <User className="h-4 w-4" />
        ) : isSystem ? (
          <AlertCircle className="h-4 w-4 text-yellow-600" />
        ) : (
          <Bot className="h-4 w-4 text-green-600" />
        )}
      </div>
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 mb-1">
          <span className="text-sm font-medium capitalize">
            {message.role}
          </span>
          <span className="text-xs text-muted-foreground">
            {new Date(message.created_at * 1000).toLocaleTimeString()}
          </span>
          {isStreaming && (
            <span className="text-xs text-green-600 animate-pulse">
              typing...
            </span>
          )}
        </div>
        <div
          className={cn(
            "rounded-lg px-4 py-3",
            isUser
              ? "bg-primary/5 border"
              : isSystem
                ? "bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800"
                : "bg-muted/50"
          )}
        >
          <p className="text-sm whitespace-pre-wrap break-words">
            {message.content}
            {isStreaming && (
              <span className="inline-block w-2 h-4 ml-1 bg-foreground animate-pulse" />
            )}
          </p>
        </div>
      </div>
    </div>
  )
}
