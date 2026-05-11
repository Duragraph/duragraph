import { Bot, User as UserIcon, Wrench } from "lucide-react"
import ReactMarkdown from "react-markdown"
import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import { Card } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { cn } from "@/lib/utils"
import type { Message } from "@/types/entities"

interface ChatMessageProps {
  message: Message
  isStreaming?: boolean
}

// Three rendered shapes:
//   * user      → right-aligned filled bubble, primary background
//   * tool      → left-aligned outline card with a Wrench badge (the
//                 tool result returned to the LLM)
//   * assistant → left-aligned card with markdown rendering; the
//                 system role uses the same shape for now since the
//                 playground rarely surfaces a system message inline
//
// Avatar lives outside the bubble (shadcn pattern) so the bubble can
// expand to full content width without the icon pinning width.

export function ChatMessage({ message, isStreaming }: ChatMessageProps) {
  const isUser = message.role === "user"
  const isTool = message.role === "tool"

  return (
    <div
      className={cn(
        "flex items-start gap-3",
        isUser && "flex-row-reverse",
      )}
    >
      <Avatar className="size-8 shrink-0 rounded-lg">
        <AvatarFallback
          className={cn(
            "rounded-lg text-xs",
            isUser
              ? "bg-primary text-primary-foreground"
              : isTool
                ? "bg-muted text-foreground"
                : "bg-secondary text-secondary-foreground",
          )}
        >
          {isUser ? (
            <UserIcon className="size-4" />
          ) : isTool ? (
            <Wrench className="size-4" />
          ) : (
            <Bot className="size-4" />
          )}
        </AvatarFallback>
      </Avatar>

      <Card
        className={cn(
          "max-w-[80%] gap-0 p-3 text-sm shadow-none",
          isUser
            ? "bg-primary text-primary-foreground"
            : isTool
              ? "border-dashed bg-muted/40"
              : undefined,
        )}
      >
        {isTool && message.name && (
          <Badge variant="outline" className="mb-2 w-fit font-mono text-[10px]">
            {message.name}
          </Badge>
        )}

        {isUser || isTool ? (
          <p className="whitespace-pre-wrap break-words">{message.content}</p>
        ) : (
          <div className="prose prose-sm max-w-none dark:prose-invert prose-p:my-1 prose-pre:my-2">
            <ReactMarkdown>{message.content}</ReactMarkdown>
          </div>
        )}

        {isStreaming && (
          <span className="ml-1 inline-block h-3 w-1.5 animate-pulse bg-foreground/60 align-middle" />
        )}
      </Card>
    </div>
  )
}
