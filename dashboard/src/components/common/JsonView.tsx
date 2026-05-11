import { useMemo } from "react"
import { Card } from "@/components/ui/card"
import { ScrollArea } from "@/components/ui/scroll-area"
import { cn } from "@/lib/utils"

// JsonView renders a syntax-highlighted, pretty-printed JSON value.
// Token classes lean on the shadcn theme tokens (foreground, primary,
// destructive, plus a couple of named accents from index.css) so the
// colours track the active theme — light / dark / future themes don't
// need separate palettes.
//
// Why hand-rolled vs react-json-view / @uiw/react-json-view:
//   * Those libraries pull in 50–80 KB minified and bring their own
//     theming system that doesn't speak the dashboard's tokens.
//   * We only need read-only display today; no collapsible nodes, no
//     edit affordances. A ~50-line tokeniser is the right size.
// If we ever need collapse + edit, swap to @uiw/react-json-view and
// thread the tokens through its `style` prop.

interface JsonViewProps {
  value: unknown
  maxHeight?: number | string
  className?: string
}

export function JsonView({
  value,
  maxHeight = 384,
  className,
}: JsonViewProps) {
  const html = useMemo(() => highlight(stringify(value, 2)), [value])
  return (
    <Card className={cn("overflow-hidden p-0", className)}>
      <ScrollArea
        style={{ maxHeight: typeof maxHeight === "number" ? `${maxHeight}px` : maxHeight }}
      >
        <pre
          className="overflow-x-auto p-4 font-mono text-xs leading-relaxed"
          dangerouslySetInnerHTML={{ __html: html }}
        />
      </ScrollArea>
    </Card>
  )
}

// JSON.stringify but tolerates undefined / functions / cyclic refs by
// replacing them with sentinel strings. Real-world `run.input` is
// rarely cyclic, but defensive: a single bad payload shouldn't blank
// the page.
function stringify(value: unknown, indent: number): string {
  const seen = new WeakSet<object>()
  return JSON.stringify(
    value,
    (_key, v) => {
      if (typeof v === "function") return "[Function]"
      if (typeof v === "undefined") return "[undefined]"
      if (typeof v === "object" && v !== null) {
        if (seen.has(v)) return "[Circular]"
        seen.add(v)
      }
      return v
    },
    indent,
  )
}

// Minimal JSON tokeniser: scans the pretty-printed source for the four
// distinct lexemes (strings, numbers, booleans/null, structural
// punctuation) and wraps each in a span with a token class. Keys
// (strings followed by `:`) are coloured separately from string
// values so they read as identifiers.
//
// The regex is the same pattern Crockford's JSON5-style highlighter
// uses; the only adaptation is matching either `key:` or bare string
// values via the trailing `\s*:` lookahead.
const TOKEN_RE =
  /("(?:\\u[a-fA-F0-9]{4}|\\[^u]|[^\\"])*"(?:\s*:)?|\b(?:true|false|null)\b|-?\d+(?:\.\d*)?(?:[eE][+-]?\d+)?)/g

function highlight(json: string): string {
  // First escape HTML so curly braces / angle brackets in string values
  // don't break the pre. Then wrap tokens.
  return json
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(TOKEN_RE, (match) => {
      let cls = "text-emerald-600 dark:text-emerald-400" // number
      if (/^"/.test(match)) {
        if (/:$/.test(match)) {
          cls = "text-primary" // key
        } else {
          cls = "text-amber-600 dark:text-amber-400" // string value
        }
      } else if (/true|false/.test(match)) {
        cls = "text-violet-600 dark:text-violet-400"
      } else if (/null/.test(match)) {
        cls = "text-muted-foreground italic"
      }
      return `<span class="${cls}">${match}</span>`
    })
}
