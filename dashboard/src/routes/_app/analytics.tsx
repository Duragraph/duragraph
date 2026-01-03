import { createFileRoute } from "@tanstack/react-router"
import { PageHeader } from "@/components/layout/PageHeader"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Activity, Zap, Clock, TrendingUp } from "lucide-react"
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
  AreaChart,
  Area,
} from "recharts"

export const Route = createFileRoute("/_app/analytics")({
  component: AnalyticsPage,
})

// Stats cards data
const stats = [
  {
    title: "Total LLM Calls",
    value: "12,847",
    change: "+12%",
    trend: "up",
    icon: Zap,
    description: "vs. last 7 days",
  },
  {
    title: "Total Tokens",
    value: "2.4M",
    change: "+8%",
    trend: "up",
    icon: Activity,
    description: "vs. last 7 days",
  },
  {
    title: "Avg Latency",
    value: "1.2s",
    change: "-15%",
    trend: "down",
    icon: Clock,
    description: "vs. last 7 days",
  },
  {
    title: "Success Rate",
    value: "98.2%",
    change: "+0.5%",
    trend: "up",
    icon: TrendingUp,
    description: "vs. last 7 days",
  },
]

// Mock data for LLM Calls Over Time
const llmCallsData = [
  { time: "00:00", calls: 120, errors: 2 },
  { time: "02:00", calls: 85, errors: 1 },
  { time: "04:00", calls: 45, errors: 0 },
  { time: "06:00", calls: 78, errors: 1 },
  { time: "08:00", calls: 245, errors: 5 },
  { time: "10:00", calls: 380, errors: 8 },
  { time: "12:00", calls: 420, errors: 6 },
  { time: "14:00", calls: 390, errors: 4 },
  { time: "16:00", calls: 350, errors: 7 },
  { time: "18:00", calls: 280, errors: 3 },
  { time: "20:00", calls: 195, errors: 2 },
  { time: "22:00", calls: 150, errors: 1 },
]

// Mock data for Token Usage
const tokenUsageData = [
  { day: "Mon", input: 45000, output: 62000 },
  { day: "Tue", input: 52000, output: 71000 },
  { day: "Wed", input: 48000, output: 65000 },
  { day: "Thu", input: 61000, output: 82000 },
  { day: "Fri", input: 55000, output: 74000 },
  { day: "Sat", input: 32000, output: 43000 },
  { day: "Sun", input: 28000, output: 38000 },
]

// Mock data for Latency Distribution
const latencyData = [
  { range: "0-0.5s", count: 2450 },
  { range: "0.5-1s", count: 4120 },
  { range: "1-1.5s", count: 3280 },
  { range: "1.5-2s", count: 1890 },
  { range: "2-3s", count: 780 },
  { range: "3-5s", count: 245 },
  { range: "5s+", count: 82 },
]

function AnalyticsPage() {
  return (
    <div>
      <PageHeader
        title="Analytics"
        description="LLM usage, token consumption, and performance metrics"
        actions={
          <Select defaultValue="7d">
            <SelectTrigger className="w-36">
              <SelectValue placeholder="Time Range" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="24h">Last 24 Hours</SelectItem>
              <SelectItem value="7d">Last 7 Days</SelectItem>
              <SelectItem value="30d">Last 30 Days</SelectItem>
              <SelectItem value="90d">Last 90 Days</SelectItem>
            </SelectContent>
          </Select>
        }
      />

      {/* Stats Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 mb-8">
        {stats.map((stat) => (
          <Card key={stat.title}>
            <CardHeader className="flex flex-row items-center justify-between pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">
                {stat.title}
              </CardTitle>
              <stat.icon className="h-4 w-4 text-muted-foreground" />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">{stat.value}</div>
              <p className="text-xs text-muted-foreground flex items-center gap-1">
                <span
                  className={
                    stat.trend === "up" ? "text-green-600" : "text-red-600"
                  }
                >
                  {stat.change}
                </span>
                {stat.description}
              </p>
            </CardContent>
          </Card>
        ))}
      </div>

      {/* Charts Section */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* LLM Calls Over Time */}
        <Card>
          <CardHeader>
            <CardTitle>LLM Calls Over Time</CardTitle>
            <CardDescription>Number of LLM API calls per hour (last 24h)</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="h-[300px]">
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={llmCallsData}>
                  <CartesianGrid strokeDasharray="3 3" className="stroke-muted" />
                  <XAxis
                    dataKey="time"
                    tick={{ fontSize: 12 }}
                    className="text-muted-foreground"
                  />
                  <YAxis tick={{ fontSize: 12 }} className="text-muted-foreground" />
                  <Tooltip
                    contentStyle={{
                      backgroundColor: "hsl(var(--background))",
                      border: "1px solid hsl(var(--border))",
                      borderRadius: "8px",
                    }}
                  />
                  <Legend />
                  <Area
                    type="monotone"
                    dataKey="calls"
                    name="Successful Calls"
                    stroke="#8b5cf6"
                    fill="#8b5cf6"
                    fillOpacity={0.3}
                  />
                  <Area
                    type="monotone"
                    dataKey="errors"
                    name="Errors"
                    stroke="#ef4444"
                    fill="#ef4444"
                    fillOpacity={0.3}
                  />
                </AreaChart>
              </ResponsiveContainer>
            </div>
          </CardContent>
        </Card>

        {/* Token Usage */}
        <Card>
          <CardHeader>
            <CardTitle>Token Usage</CardTitle>
            <CardDescription>Input vs Output tokens by day</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="h-[300px]">
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={tokenUsageData}>
                  <CartesianGrid strokeDasharray="3 3" className="stroke-muted" />
                  <XAxis
                    dataKey="day"
                    tick={{ fontSize: 12 }}
                    className="text-muted-foreground"
                  />
                  <YAxis
                    tick={{ fontSize: 12 }}
                    className="text-muted-foreground"
                    tickFormatter={(value) => `${(value / 1000).toFixed(0)}k`}
                  />
                  <Tooltip
                    contentStyle={{
                      backgroundColor: "hsl(var(--background))",
                      border: "1px solid hsl(var(--border))",
                      borderRadius: "8px",
                    }}
                    formatter={(value) => (value as number).toLocaleString()}
                  />
                  <Legend />
                  <Bar dataKey="input" name="Input Tokens" fill="#3b82f6" radius={[4, 4, 0, 0]} />
                  <Bar dataKey="output" name="Output Tokens" fill="#22c55e" radius={[4, 4, 0, 0]} />
                </BarChart>
              </ResponsiveContainer>
            </div>
          </CardContent>
        </Card>

        {/* Latency Distribution */}
        <Card>
          <CardHeader>
            <CardTitle>Latency Distribution</CardTitle>
            <CardDescription>Response time distribution</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="h-[300px]">
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={latencyData}>
                  <CartesianGrid strokeDasharray="3 3" className="stroke-muted" />
                  <XAxis
                    dataKey="range"
                    tick={{ fontSize: 11 }}
                    className="text-muted-foreground"
                  />
                  <YAxis tick={{ fontSize: 12 }} className="text-muted-foreground" />
                  <Tooltip
                    contentStyle={{
                      backgroundColor: "hsl(var(--background))",
                      border: "1px solid hsl(var(--border))",
                      borderRadius: "8px",
                    }}
                    formatter={(value) => [(value as number).toLocaleString(), "Requests"]}
                  />
                  <Bar dataKey="count" name="Requests" fill="#f59e0b" radius={[4, 4, 0, 0]} />
                </BarChart>
              </ResponsiveContainer>
            </div>
          </CardContent>
        </Card>

        {/* Model Usage */}
        <Card>
          <CardHeader>
            <CardTitle>Model Usage</CardTitle>
            <CardDescription>Calls per LLM model</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-4">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <div className="w-3 h-3 rounded-full bg-purple-500" />
                  <span className="text-sm">gpt-4</span>
                </div>
                <div className="flex items-center gap-2">
                  <div className="w-32 h-2 bg-muted rounded-full overflow-hidden">
                    <div className="w-3/4 h-full bg-purple-500" />
                  </div>
                  <span className="text-sm font-mono">8,234</span>
                </div>
              </div>
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <div className="w-3 h-3 rounded-full bg-blue-500" />
                  <span className="text-sm">claude-3-sonnet</span>
                </div>
                <div className="flex items-center gap-2">
                  <div className="w-32 h-2 bg-muted rounded-full overflow-hidden">
                    <div className="w-1/2 h-full bg-blue-500" />
                  </div>
                  <span className="text-sm font-mono">3,102</span>
                </div>
              </div>
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <div className="w-3 h-3 rounded-full bg-green-500" />
                  <span className="text-sm">gpt-4o-mini</span>
                </div>
                <div className="flex items-center gap-2">
                  <div className="w-32 h-2 bg-muted rounded-full overflow-hidden">
                    <div className="w-1/4 h-full bg-green-500" />
                  </div>
                  <span className="text-sm font-mono">1,511</span>
                </div>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
