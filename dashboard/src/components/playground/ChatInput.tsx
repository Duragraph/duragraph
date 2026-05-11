import { useState, useCallback } from 'react'

interface ChatInputProps {
  onSend: (content: string) => void
  disabled: boolean
  isStreaming: boolean
}

export function ChatInput({ onSend, disabled, isStreaming }: ChatInputProps) {
  const [value, setValue] = useState('')

  const handleSubmit = useCallback(() => {
    const trimmed = value.trim()
    if (!trimmed || disabled) return
    onSend(trimmed)
    setValue('')
  }, [value, disabled, onSend])

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault()
        handleSubmit()
      }
    },
    [handleSubmit],
  )

  return (
    <div className="border-t border-border p-4">
      <div className="mx-auto flex max-w-3xl gap-2">
        <textarea
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={
            disabled
              ? isStreaming
                ? 'Waiting for response...'
                : 'Select an assistant to start'
              : 'Type your message...'
          }
          disabled={disabled}
          rows={1}
          className="flex-1 resize-none border border-input bg-background px-4 py-2.5 text-sm placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring disabled:opacity-50"
        />
        <button
          onClick={handleSubmit}
          disabled={disabled || !value.trim()}
          className="bg-primary px-5 py-2.5 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
        >
          Send
        </button>
      </div>
    </div>
  )
}
