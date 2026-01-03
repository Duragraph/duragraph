import { createFileRoute } from "@tanstack/react-router"
import { PageHeader } from "@/components/layout/PageHeader"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Progress } from "@/components/ui/progress"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { DollarSign, TrendingUp, AlertCircle, Settings } from "lucide-react"
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Cell,
} from "recharts"

export const Route = createFileRoute("/_app/costs")({
  component: CostsPage,
})

// Mock data
const costStats = [
  {
    title: "Total Spend (This Month)",
    value: "$1,247.52",
    change: "+18%",
    icon: DollarSign,
    description: "vs. last month",
  },
  {
    title: "Daily Average",
    value: "$41.58",
    change: "+5%",
    icon: TrendingUp,
    description: "vs. 30-day avg",
  },
  {
    title: "Budget Remaining",
    value: "$752.48",
    remaining: 37.6,
    icon: AlertCircle,
    description: "of $2,000 monthly budget",
  },
]

// Mock data for Daily Spend
const dailySpendData = [
  { date: "Dec 1", spend: 38.5 },
  { date: "Dec 2", spend: 42.3 },
  { date: "Dec 3", spend: 35.8 },
  { date: "Dec 4", spend: 51.2 },
  { date: "Dec 5", spend: 48.7 },
  { date: "Dec 6", spend: 29.4 },
  { date: "Dec 7", spend: 31.2 },
  { date: "Dec 8", spend: 45.6 },
  { date: "Dec 9", spend: 52.8 },
  { date: "Dec 10", spend: 41.3 },
  { date: "Dec 11", spend: 38.9 },
  { date: "Dec 12", spend: 55.2 },
  { date: "Dec 13", spend: 47.8 },
  { date: "Dec 14", spend: 33.5 },
]

const topRuns = [
  { id: "run_abc123", graph: "customer_support", tokens: 45231, cost: 1.52 },
  { id: "run_def456", graph: "code_reviewer", tokens: 38921, cost: 1.31 },
  { id: "run_ghi789", graph: "data_analyst", tokens: 32145, cost: 1.08 },
  { id: "run_jkl012", graph: "sales_agent", tokens: 28934, cost: 0.97 },
  { id: "run_mno345", graph: "customer_support", tokens: 25123, cost: 0.85 },
]

const costByModel = [
  { model: "gpt-4", calls: 8234, tokens: "1.2M", cost: 847.32, percentage: 68 },
  { model: "claude-3-sonnet", calls: 3102, tokens: "450K", cost: 287.45, percentage: 23 },
  { model: "gpt-4o-mini", calls: 1511, tokens: "200K", cost: 112.75, percentage: 9 },
]

// Average daily spend for comparison
const avgDailySpend = 41.58

function CostsPage() {
  return (
    <div>
      <PageHeader
        title="Costs"
        description="Track and manage LLM API spending"
        actions={
          <>
            <Select defaultValue="month">
              <SelectTrigger className="w-40">
                <SelectValue placeholder="Time Range" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="week">This Week</SelectItem>
                <SelectItem value="month">This Month</SelectItem>
                <SelectItem value="quarter">This Quarter</SelectItem>
                <SelectItem value="year">This Year</SelectItem>
              </SelectContent>
            </Select>
            <Button variant="outline" size="sm">
              <Settings className="h-4 w-4 mr-2" />
              Budgets
            </Button>
          </>
        }
      />

      {/* Cost Stats */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-8">
        {costStats.map((stat) => (
          <Card key={stat.title}>
            <CardHeader className="flex flex-row items-center justify-between pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">
                {stat.title}
              </CardTitle>
              <stat.icon className="h-4 w-4 text-muted-foreground" />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">{stat.value}</div>
              {"remaining" in stat && stat.remaining !== undefined ? (
                <div className="mt-2">
                  <Progress value={100 - stat.remaining} className="h-2" />
                  <p className="text-xs text-muted-foreground mt-1">
                    {stat.description}
                  </p>
                </div>
              ) : (
                <p className="text-xs text-muted-foreground">
                  <span className="text-green-600">{stat.change}</span> {stat.description}
                </p>
              )}
            </CardContent>
          </Card>
        ))}
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Daily Spend Chart */}
        <Card>
          <CardHeader>
            <CardTitle>Daily Spend</CardTitle>
            <CardDescription>Cost breakdown by day</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="h-[250px]">
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={dailySpendData}>
                  <CartesianGrid strokeDasharray="3 3" className="stroke-muted" />
                  <XAxis
                    dataKey="date"
                    tick={{ fontSize: 11 }}
                    className="text-muted-foreground"
                  />
                  <YAxis
                    tick={{ fontSize: 12 }}
                    className="text-muted-foreground"
                    tickFormatter={(value) => `$${value}`}
                  />
                  <Tooltip
                    contentStyle={{
                      backgroundColor: "hsl(var(--background))",
                      border: "1px solid hsl(var(--border))",
                      borderRadius: "8px",
                    }}
                    formatter={(value) => [`$${(value as number).toFixed(2)}`, "Spend"]}
                  />
                  <Bar dataKey="spend" radius={[4, 4, 0, 0]}>
                    {dailySpendData.map((entry, index) => (
                      <Cell
                        key={`cell-${index}`}
                        fill={entry.spend > avgDailySpend ? "#f59e0b" : "#22c55e"}
                      />
                    ))}
                  </Bar>
                </BarChart>
              </ResponsiveContainer>
            </div>
          </CardContent>
        </Card>

        {/* Cost by Model */}
        <Card>
          <CardHeader>
            <CardTitle>Cost by Model</CardTitle>
            <CardDescription>Spending distribution across LLM providers</CardDescription>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Model</TableHead>
                  <TableHead className="text-right">Calls</TableHead>
                  <TableHead className="text-right">Tokens</TableHead>
                  <TableHead className="text-right">Cost</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {costByModel.map((row) => (
                  <TableRow key={row.model}>
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <div
                          className="w-2 h-2 rounded-full"
                          style={{
                            backgroundColor:
                              row.model === "gpt-4"
                                ? "#8b5cf6"
                                : row.model === "claude-3-sonnet"
                                  ? "#3b82f6"
                                  : "#22c55e",
                          }}
                        />
                        <span className="font-medium">{row.model}</span>
                      </div>
                    </TableCell>
                    <TableCell className="text-right font-mono">
                      {row.calls.toLocaleString()}
                    </TableCell>
                    <TableCell className="text-right font-mono">
                      {row.tokens}
                    </TableCell>
                    <TableCell className="text-right font-mono font-medium">
                      ${row.cost.toFixed(2)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>

        {/* Top Expensive Runs */}
        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle>Top Expensive Runs</CardTitle>
            <CardDescription>Runs with highest token consumption</CardDescription>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Run ID</TableHead>
                  <TableHead>Graph</TableHead>
                  <TableHead className="text-right">Tokens</TableHead>
                  <TableHead className="text-right">Cost</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {topRuns.map((run) => (
                  <TableRow key={run.id} className="cursor-pointer hover:bg-muted/50">
                    <TableCell className="font-mono">{run.id}</TableCell>
                    <TableCell>{run.graph}</TableCell>
                    <TableCell className="text-right font-mono">
                      {run.tokens.toLocaleString()}
                    </TableCell>
                    <TableCell className="text-right font-mono font-medium">
                      ${run.cost.toFixed(2)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
