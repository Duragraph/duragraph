import { createFileRoute, Link } from "@tanstack/react-router"
import { PageHeader } from "@/components/layout/PageHeader"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { ScrollArea } from "@/components/ui/scroll-area"
import {
  ArrowLeft,
  CheckCircle2,
  Clock,
  Zap,
  DollarSign,
  MessageSquare,
  Wrench,
  ChevronRight,
} from "lucide-react"
import { useState } from "react"

export const Route = createFileRoute("/_app/traces/$traceId")({
  component: TraceDetailPage,
})

// Mock trace data
const mockTrace = {
  id: "run_abc123",
  graph: "customer_support",
  model: "gpt-4",
  status: "success",
  startedAt: "2025-01-15T10:30:00.000Z",
  completedAt: "2025-01-15T10:30:02.300Z",
  duration: "2.3s",
  tokens: {
    input: 234,
    output: 567,
    total: 801,
  },
  cost: 0.024,
  spans: [
    {
      id: "span_1",
      name: "classify",
      type: "llm",
      startTime: 0,
      duration: 120,
      tokens: { input: 50, output: 20 },
      model: "gpt-4",
      prompt: "Classify the following customer inquiry:\n\nUser: I need help with my order #12345",
      response: "intent: order_inquiry\nconfidence: 0.95",
    },
    {
      id: "span_2",
      name: "router",
      type: "router",
      startTime: 120,
      duration: 25,
    },
    {
      id: "span_3",
      name: "lookup_order",
      type: "tool",
      startTime: 145,
      duration: 200,
      tool: "order_lookup",
      input: { order_id: "12345" },
      output: { status: "shipped", eta: "2025-01-17" },
    },
    {
      id: "span_4",
      name: "support",
      type: "llm",
      startTime: 345,
      duration: 890,
      tokens: { input: 184, output: 547 },
      model: "gpt-4",
      prompt: "Based on the order status...",
      response: "I found your order #12345. It shipped yesterday and should arrive by Friday.",
    },
  ],
}

function SpanIcon({ type }: { type: string }) {
  switch (type) {
    case "llm":
      return <Zap className="w-4 h-4 text-purple-500" />
    case "tool":
      return <Wrench className="w-4 h-4 text-amber-500" />
    case "router":
      return <ChevronRight className="w-4 h-4 text-blue-500" />
    default:
      return <MessageSquare className="w-4 h-4 text-gray-500" />
  }
}

function SpanRow({ span, onSelect, isSelected }: { span: any; onSelect: () => void; isSelected: boolean }) {
  const widthPercent = (span.duration / 2300) * 100
  const leftPercent = (span.startTime / 2300) * 100

  return (
    <div
      className={`p-3 border-b cursor-pointer hover:bg-muted/50 ${isSelected ? "bg-muted" : ""}`}
      onClick={onSelect}
    >
      <div className="flex items-center gap-3 mb-2">
        <SpanIcon type={span.type} />
        <span className="font-medium">{span.name}</span>
        <Badge variant="outline" className="text-xs">
          {span.type}
        </Badge>
        <span className="text-sm text-muted-foreground ml-auto">
          {span.duration}ms
        </span>
      </div>
      {/* Timeline bar */}
      <div className="h-2 bg-muted rounded-full overflow-hidden relative">
        <div
          className={`absolute h-full rounded-full ${
            span.type === "llm" ? "bg-purple-500" : span.type === "tool" ? "bg-amber-500" : "bg-blue-500"
          }`}
          style={{
            left: `${leftPercent}%`,
            width: `${widthPercent}%`,
          }}
        />
      </div>
    </div>
  )
}

function TraceDetailPage() {
  const { traceId } = Route.useParams()
  const [selectedSpan, setSelectedSpan] = useState<any>(mockTrace.spans[0])

  return (
    <div>
      <div className="mb-4">
        <Link
          to="/traces"
          className="text-sm text-muted-foreground hover:text-foreground inline-flex items-center gap-1"
        >
          <ArrowLeft className="w-4 h-4" />
          Back to Traces
        </Link>
      </div>

      <PageHeader
        title={
          <div className="flex items-center gap-3">
            <span className="font-mono">{traceId}</span>
            <Badge variant="outline" className="text-green-600 border-green-600">
              <CheckCircle2 className="w-3 h-3 mr-1" />
              Success
            </Badge>
          </div>
        }
        description={`Graph: ${mockTrace.graph} · Model: ${mockTrace.model}`}
      />

      {/* Stats */}
      <div className="grid grid-cols-4 gap-4 mb-6">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground flex items-center gap-2">
              <Clock className="h-4 w-4" />
              Duration
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{mockTrace.duration}</div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground flex items-center gap-2">
              <Zap className="h-4 w-4" />
              LLM Calls
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {mockTrace.spans.filter((s) => s.type === "llm").length}
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground flex items-center gap-2">
              <MessageSquare className="h-4 w-4" />
              Tokens
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{mockTrace.tokens.total.toLocaleString()}</div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground flex items-center gap-2">
              <DollarSign className="h-4 w-4" />
              Cost
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">${mockTrace.cost.toFixed(3)}</div>
          </CardContent>
        </Card>
      </div>

      {/* Waterfall Timeline + Details */}
      <div className="grid grid-cols-2 gap-6">
        {/* Timeline */}
        <Card>
          <CardHeader>
            <CardTitle>Timeline</CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            <ScrollArea className="h-[500px]">
              {mockTrace.spans.map((span) => (
                <SpanRow
                  key={span.id}
                  span={span}
                  onSelect={() => setSelectedSpan(span)}
                  isSelected={selectedSpan?.id === span.id}
                />
              ))}
            </ScrollArea>
          </CardContent>
        </Card>

        {/* Details Panel */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <SpanIcon type={selectedSpan?.type} />
              {selectedSpan?.name}
            </CardTitle>
          </CardHeader>
          <CardContent>
            {selectedSpan?.type === "llm" && (
              <Tabs defaultValue="prompt">
                <TabsList>
                  <TabsTrigger value="prompt">Prompt</TabsTrigger>
                  <TabsTrigger value="response">Response</TabsTrigger>
                  <TabsTrigger value="metadata">Metadata</TabsTrigger>
                </TabsList>
                <TabsContent value="prompt" className="mt-4">
                  <ScrollArea className="h-[350px]">
                    <pre className="bg-muted p-4 rounded-lg text-sm whitespace-pre-wrap">
                      {selectedSpan.prompt}
                    </pre>
                  </ScrollArea>
                </TabsContent>
                <TabsContent value="response" className="mt-4">
                  <ScrollArea className="h-[350px]">
                    <pre className="bg-muted p-4 rounded-lg text-sm whitespace-pre-wrap">
                      {selectedSpan.response}
                    </pre>
                  </ScrollArea>
                </TabsContent>
                <TabsContent value="metadata" className="mt-4">
                  <div className="space-y-3">
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">Model</span>
                      <span>{selectedSpan.model}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">Duration</span>
                      <span>{selectedSpan.duration}ms</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">Input Tokens</span>
                      <span>{selectedSpan.tokens?.input}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">Output Tokens</span>
                      <span>{selectedSpan.tokens?.output}</span>
                    </div>
                  </div>
                </TabsContent>
              </Tabs>
            )}

            {selectedSpan?.type === "tool" && (
              <Tabs defaultValue="input">
                <TabsList>
                  <TabsTrigger value="input">Input</TabsTrigger>
                  <TabsTrigger value="output">Output</TabsTrigger>
                </TabsList>
                <TabsContent value="input" className="mt-4">
                  <pre className="bg-muted p-4 rounded-lg text-sm">
                    {JSON.stringify(selectedSpan.input, null, 2)}
                  </pre>
                </TabsContent>
                <TabsContent value="output" className="mt-4">
                  <pre className="bg-muted p-4 rounded-lg text-sm">
                    {JSON.stringify(selectedSpan.output, null, 2)}
                  </pre>
                </TabsContent>
              </Tabs>
            )}

            {selectedSpan?.type === "router" && (
              <div className="space-y-3">
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Type</span>
                  <span>Conditional Router</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Duration</span>
                  <span>{selectedSpan.duration}ms</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Next Node</span>
                  <span>lookup_order</span>
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
