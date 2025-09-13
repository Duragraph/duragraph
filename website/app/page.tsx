import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import {
  ArrowRight,
  Github,
  Star,
  Users,
  Zap,
  Shield,
  Code,
  Database,
  Brain,
  BarChart3,
  GitBranch,
  Play,
} from "lucide-react"

export default function HomePage() {
  return (
    <div className="min-h-screen bg-[#0D1117] text-[#C9D1D9]">
      {/* Hero Section */}
      <section className="relative overflow-hidden">
        <div className="absolute inset-0 bg-gradient-to-br from-[#6C63FF]/20 via-transparent to-[#00C6A7]/20" />
        <div className="relative max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 pt-20 pb-32">
          <div className="text-center">
            <div className="mb-8">
              <Badge variant="secondary" className="bg-[#161B22] text-[#6C63FF] border-[#6C63FF]/30 mb-4">
                Open Source • Enterprise Ready
              </Badge>
            </div>
            <h1 className="text-5xl md:text-7xl font-bold mb-6 text-balance">
              <span className="text-white">Durable Workflows.</span>
              <br />
              <span className="text-[#6C63FF]">Open Source.</span>
              <br />
              <span className="text-[#00C6A7]">Cloud Ready.</span>
            </h1>
            <p className="text-xl md:text-2xl text-[#8B949E] mb-12 max-w-4xl mx-auto text-pretty">
              Duragraph is an open-core orchestration platform for AI and data workflows — self-host or run on Duragraph
              Cloud.
            </p>
            <div className="flex flex-col sm:flex-row gap-4 justify-center">
              <Button size="lg" className="bg-[#6C63FF] hover:bg-[#6C63FF]/90 text-white px-8 py-4 text-lg">
                Get Started (OSS)
                <ArrowRight className="ml-2 h-5 w-5" />
              </Button>
              <Button
                size="lg"
                variant="outline"
                className="border-[#00C6A7] text-[#00C6A7] hover:bg-[#00C6A7]/10 px-8 py-4 text-lg bg-transparent"
              >
                Try Duragraph Cloud
              </Button>
            </div>
          </div>

          {/* Animated Workflow Diagram */}
          <div className="mt-20 relative">
            <div className="bg-[#161B22] border border-[#30363D] p-8 relative overflow-hidden">
              {/* Background grid pattern */}
              <div className="absolute inset-0 opacity-10">
                <div
                  className="absolute inset-0"
                  style={{
                    backgroundImage: `radial-gradient(circle at 1px 1px, rgba(108, 99, 255, 0.3) 1px, transparent 0)`,
                    backgroundSize: "20px 20px",
                  }}
                />
              </div>

              {/* Main workflow container */}
              <div className="relative">
                <div className="flex items-center justify-between max-w-2xl mx-auto">
                  {/* Step 1: Trigger */}
                  <div className="flex flex-col items-center group">
                    <div className="relative">
                      <div className="w-16 h-16 bg-[#6C63FF] border-4 border-[#6C63FF]/30 flex items-center justify-center relative overflow-hidden animate-pulse">
                        <Play className="h-8 w-8 text-white relative z-10" />
                        {/* Ripple effect */}
                        <div className="absolute inset-0 bg-[#6C63FF]/20 animate-ping" />
                        <div
                          className="absolute inset-0 bg-[#6C63FF]/10 animate-ping"
                          style={{ animationDelay: "0.5s" }}
                        />
                      </div>
                      {/* Glow effect */}
                      <div className="absolute inset-0 w-16 h-16 bg-[#6C63FF]/30 blur-xl animate-pulse" />
                    </div>
                    <span className="text-xs text-[#8B949E] mt-2 font-mono">TRIGGER</span>
                  </div>

                  {/* Connection 1 with flowing data */}
                  <div className="flex-1 relative mx-4">
                    <div className="h-0.5 bg-gradient-to-r from-[#6C63FF] via-[#6C63FF]/50 to-transparent relative overflow-hidden">
                      {/* Flowing data particles */}
                      <div
                        className="absolute top-0 left-0 w-2 h-0.5 bg-[#6C63FF] animate-pulse"
                        style={{ animation: "flow 2s linear infinite" }}
                      />
                      <div
                        className="absolute top-0 left-0 w-1 h-0.5 bg-white/80"
                        style={{ animation: "flow 2s linear infinite 0.5s" }}
                      />
                    </div>
                    <ArrowRight className="absolute top-1/2 right-0 transform -translate-y-1/2 h-4 w-4 text-[#6C63FF] animate-pulse" />
                  </div>

                  {/* Step 2: Process */}
                  <div className="flex flex-col items-center group">
                    <div className="relative">
                      <div
                        className="w-16 h-16 bg-[#00C6A7] border-4 border-[#00C6A7]/30 flex items-center justify-center relative overflow-hidden"
                        style={{ animation: "pulse 2s ease-in-out infinite 1s" }}
                      >
                        <GitBranch className="h-8 w-8 text-white relative z-10 transform transition-transform group-hover:rotate-12" />
                        {/* Processing indicator */}
                        <div
                          className="absolute inset-2 border-2 border-white/30 animate-spin"
                          style={{ animationDuration: "3s" }}
                        />
                      </div>
                      <div
                        className="absolute inset-0 w-16 h-16 bg-[#00C6A7]/30 blur-xl"
                        style={{ animation: "pulse 2s ease-in-out infinite 1s" }}
                      />
                    </div>
                    <span className="text-xs text-[#8B949E] mt-2 font-mono">PROCESS</span>
                  </div>

                  {/* Connection 2 with flowing data */}
                  <div className="flex-1 relative mx-4">
                    <div className="h-0.5 bg-gradient-to-r from-[#00C6A7] via-[#00C6A7]/50 to-transparent relative overflow-hidden">
                      <div
                        className="absolute top-0 left-0 w-2 h-0.5 bg-[#00C6A7]"
                        style={{ animation: "flow 2s linear infinite 1.5s" }}
                      />
                      <div
                        className="absolute top-0 left-0 w-1 h-0.5 bg-white/80"
                        style={{ animation: "flow 2s linear infinite 2s" }}
                      />
                    </div>
                    <ArrowRight
                      className="absolute top-1/2 right-0 transform -translate-y-1/2 h-4 w-4 text-[#00C6A7]"
                      style={{ animation: "pulse 2s ease-in-out infinite 1.5s" }}
                    />
                  </div>

                  {/* Step 3: Complete */}
                  <div className="flex flex-col items-center group">
                    <div className="relative">
                      <div
                        className="w-16 h-16 bg-[#FF7A59] border-4 border-[#FF7A59]/30 flex items-center justify-center relative overflow-hidden"
                        style={{ animation: "pulse 2s ease-in-out infinite 2s" }}
                      >
                        <BarChart3 className="h-8 w-8 text-white relative z-10 transform transition-transform group-hover:scale-110" />
                        {/* Success indicator */}
                        <div
                          className="absolute inset-0 bg-gradient-to-r from-transparent via-white/20 to-transparent transform -skew-x-12 animate-pulse"
                          style={{ animation: "shimmer 3s ease-in-out infinite 2s" }}
                        />
                      </div>
                      <div
                        className="absolute inset-0 w-16 h-16 bg-[#FF7A59]/30 blur-xl"
                        style={{ animation: "pulse 2s ease-in-out infinite 2s" }}
                      />
                    </div>
                    <span className="text-xs text-[#8B949E] mt-2 font-mono">COMPLETE</span>
                  </div>
                </div>

                {/* Status indicator */}
                <div className="mt-8 text-center">
                  <div className="inline-flex items-center space-x-2 bg-[#0D1117] px-4 py-2 border border-[#30363D]">
                    <div className="w-2 h-2 bg-[#3FB950] rounded-full animate-pulse" />
                    <span className="text-sm text-[#8B949E] font-mono">Workflow orchestration in real-time</span>
                    <div className="flex space-x-1">
                      <div className="w-1 h-4 bg-[#6C63FF] animate-pulse" style={{ animationDelay: "0s" }} />
                      <div className="w-1 h-4 bg-[#00C6A7] animate-pulse" style={{ animationDelay: "0.2s" }} />
                      <div className="w-1 h-4 bg-[#FF7A59] animate-pulse" style={{ animationDelay: "0.4s" }} />
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* Why Duragraph */}
      <section className="py-24 bg-[#161B22]">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
          <div className="text-center mb-16">
            <h2 className="text-4xl font-bold text-white mb-4">Why Duragraph</h2>
            <p className="text-xl text-[#8B949E] max-w-3xl mx-auto">
              Built for developers who need reliable, scalable workflow orchestration
            </p>
          </div>
          <div className="grid md:grid-cols-3 gap-8">
            <Card className="bg-[#0D1117] border-[#30363D]">
              <CardContent className="p-8 text-center">
                <Shield className="h-12 w-12 text-[#6C63FF] mx-auto mb-4" />
                <h3 className="text-xl font-semibold text-white mb-3">Resilient</h3>
                <p className="text-[#8B949E]">
                  Fault-tolerant, stateful workflows that survive failures and maintain consistency across distributed
                  systems.
                </p>
              </CardContent>
            </Card>
            <Card className="bg-[#0D1117] border-[#30363D]">
              <CardContent className="p-8 text-center">
                <Code className="h-12 w-12 text-[#00C6A7] mx-auto mb-4" />
                <h3 className="text-xl font-semibold text-white mb-3">Developer-Friendly</h3>
                <p className="text-[#8B949E]">
                  Graph APIs, LangGraph compatible, with multi-language SDKs for Python, Go, and TypeScript.
                </p>
              </CardContent>
            </Card>
            <Card className="bg-[#0D1117] border-[#30363D]">
              <CardContent className="p-8 text-center">
                <Zap className="h-12 w-12 text-[#FF7A59] mx-auto mb-4" />
                <h3 className="text-xl font-semibold text-white mb-3">Cloud-Scale</h3>
                <p className="text-[#8B949E]">
                  Managed hosting with enterprise features, real-time monitoring, and OpenTelemetry tracing.
                </p>
              </CardContent>
            </Card>
          </div>
        </div>
      </section>

      {/* How It Works */}
      <section className="py-24">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
          <div className="text-center mb-16">
            <h2 className="text-4xl font-bold text-white mb-4">How It Works</h2>
            <p className="text-xl text-[#8B949E]">From graph definition to production deployment in minutes</p>
          </div>
          <div className="grid md:grid-cols-4 gap-8">
            <div className="text-center">
              <div className="w-16 h-16 bg-[#6C63FF] rounded-full flex items-center justify-center mx-auto mb-4">
                <span className="text-white font-bold text-xl">1</span>
              </div>
              <h3 className="text-lg font-semibold text-white mb-2">Define Graph</h3>
              <p className="text-[#8B949E]">Create workflows using familiar graph APIs</p>
            </div>
            <div className="text-center">
              <div className="w-16 h-16 bg-[#00C6A7] rounded-full flex items-center justify-center mx-auto mb-4">
                <span className="text-white font-bold text-xl">2</span>
              </div>
              <h3 className="text-lg font-semibold text-white mb-2">Generate Code</h3>
              <p className="text-[#8B949E]">Auto-generate durable workflow implementations</p>
            </div>
            <div className="text-center">
              <div className="w-16 h-16 bg-[#FF7A59] rounded-full flex items-center justify-center mx-auto mb-4">
                <span className="text-white font-bold text-xl">3</span>
              </div>
              <h3 className="text-lg font-semibold text-white mb-2">Run Workflows</h3>
              <p className="text-[#8B949E]">Execute with fault tolerance and state management</p>
            </div>
            <div className="text-center">
              <div className="w-16 h-16 bg-[#3FB950] rounded-full flex items-center justify-center mx-auto mb-4">
                <span className="text-white font-bold text-xl">4</span>
              </div>
              <h3 className="text-lg font-semibold text-white mb-2">Monitor</h3>
              <p className="text-[#8B949E]">Real-time observability and debugging</p>
            </div>
          </div>
        </div>
      </section>

      {/* Open Source + Cloud */}
      <section className="py-24 bg-[#161B22]">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
          <div className="text-center mb-16">
            <h2 className="text-4xl font-bold text-white mb-4">Open-Source Core + Cloud Upgrade</h2>
          </div>
          <div className="grid md:grid-cols-2 gap-8">
            <Card className="bg-[#0D1117] border-[#30363D]">
              <CardContent className="p-8">
                <div className="flex items-center mb-4">
                  <Github className="h-8 w-8 text-[#6C63FF] mr-3" />
                  <h3 className="text-2xl font-bold text-white">Duragraph OSS</h3>
                </div>
                <p className="text-[#8B949E] mb-6">Apache-licensed, self-host anywhere.</p>
                <ul className="space-y-3 mb-8">
                  <li className="flex items-center text-[#C9D1D9]">
                    <div className="w-2 h-2 bg-[#6C63FF] rounded-full mr-3" />
                    Core orchestration engine
                  </li>
                  <li className="flex items-center text-[#C9D1D9]">
                    <div className="w-2 h-2 bg-[#6C63FF] rounded-full mr-3" />
                    Multi-language SDKs
                  </li>
                  <li className="flex items-center text-[#C9D1D9]">
                    <div className="w-2 h-2 bg-[#6C63FF] rounded-full mr-3" />
                    LangGraph compatibility
                  </li>
                  <li className="flex items-center text-[#C9D1D9]">
                    <div className="w-2 h-2 bg-[#6C63FF] rounded-full mr-3" />
                    Community support
                  </li>
                </ul>
                <Button
                  variant="outline"
                  className="w-full border-[#6C63FF] text-[#6C63FF] hover:bg-[#6C63FF]/10 bg-transparent"
                >
                  Get Started
                </Button>
              </CardContent>
            </Card>
            <Card className="bg-[#0D1117] border-[#00C6A7]">
              <CardContent className="p-8">
                <div className="flex items-center mb-4">
                  <Zap className="h-8 w-8 text-[#00C6A7] mr-3" />
                  <h3 className="text-2xl font-bold text-white">Duragraph Cloud</h3>
                </div>
                <p className="text-[#8B949E] mb-6">Fully managed orchestration with RBAC, audit logs, team features.</p>
                <ul className="space-y-3 mb-8">
                  <li className="flex items-center text-[#C9D1D9]">
                    <div className="w-2 h-2 bg-[#00C6A7] rounded-full mr-3" />
                    Managed hosting & scaling
                  </li>
                  <li className="flex items-center text-[#C9D1D9]">
                    <div className="w-2 h-2 bg-[#00C6A7] rounded-full mr-3" />
                    Enterprise security & RBAC
                  </li>
                  <li className="flex items-center text-[#C9D1D9]">
                    <div className="w-2 h-2 bg-[#00C6A7] rounded-full mr-3" />
                    Advanced monitoring & alerts
                  </li>
                  <li className="flex items-center text-[#C9D1D9]">
                    <div className="w-2 h-2 bg-[#00C6A7] rounded-full mr-3" />
                    SLA-backed uptime
                  </li>
                </ul>
                <Button className="w-full bg-[#00C6A7] hover:bg-[#00C6A7]/90 text-white">Try Cloud</Button>
              </CardContent>
            </Card>
          </div>
          <div className="text-center mt-8">
            <Button variant="link" className="text-[#8B949E] hover:text-[#C9D1D9]">
              Compare Plans →
            </Button>
          </div>
        </div>
      </section>

      {/* Use Cases */}
      <section className="py-24">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
          <div className="text-center mb-16">
            <h2 className="text-4xl font-bold text-white mb-4">Use Cases</h2>
            <p className="text-xl text-[#8B949E]">Powering the next generation of AI and data applications</p>
          </div>
          <div className="grid md:grid-cols-2 lg:grid-cols-4 gap-6">
            <Card className="bg-[#161B22] border-[#30363D]">
              <CardContent className="p-6">
                <Brain className="h-8 w-8 text-[#6C63FF] mb-3" />
                <h3 className="text-lg font-semibold text-white mb-2">AI Agents</h3>
                <p className="text-[#8B949E] text-sm">Long-running conversational memory, retrieval workflows.</p>
              </CardContent>
            </Card>
            <Card className="bg-[#161B22] border-[#30363D]">
              <CardContent className="p-6">
                <Database className="h-8 w-8 text-[#00C6A7] mb-3" />
                <h3 className="text-lg font-semibold text-white mb-2">Data Pipelines</h3>
                <p className="text-[#8B949E] text-sm">ETL, batch, and streaming data processing.</p>
              </CardContent>
            </Card>
            <Card className="bg-[#161B22] border-[#30363D]">
              <CardContent className="p-6">
                <BarChart3 className="h-8 w-8 text-[#FF7A59] mb-3" />
                <h3 className="text-lg font-semibold text-white mb-2">Research Labs</h3>
                <p className="text-[#8B949E] text-sm">Reproducibility & traceability for experiments.</p>
              </CardContent>
            </Card>
            <Card className="bg-[#161B22] border-[#30363D]">
              <CardContent className="p-6">
                <Users className="h-8 w-8 text-[#3FB950] mb-3" />
                <h3 className="text-lg font-semibold text-white mb-2">Enterprise Apps</h3>
                <p className="text-[#8B949E] text-sm">SLA-backed business workflows at scale.</p>
              </CardContent>
            </Card>
          </div>
        </div>
      </section>

      {/* Community */}
      <section className="py-24 bg-[#161B22]">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 text-center">
          <h2 className="text-4xl font-bold text-white mb-4">Community + Docs</h2>
          <p className="text-xl text-[#8B949E] mb-12">
            Join developers building the next generation of AI orchestration
          </p>
          <div className="flex flex-col sm:flex-row gap-4 justify-center">
            <Button variant="outline" className="border-[#8B949E] text-[#8B949E] hover:bg-[#8B949E]/10 bg-transparent">
              <Github className="mr-2 h-5 w-5" />
              GitHub
              <Star className="ml-2 h-4 w-4" />
            </Button>
            <Button variant="outline" className="border-[#8B949E] text-[#8B949E] hover:bg-[#8B949E]/10 bg-transparent">
              <Users className="mr-2 h-5 w-5" />
              Discord Community
            </Button>
            <Button variant="outline" className="border-[#8B949E] text-[#8B949E] hover:bg-[#8B949E]/10 bg-transparent">
              📚 Documentation
            </Button>
          </div>
        </div>
      </section>

      {/* Footer */}
      <footer className="py-16 border-t border-[#30363D]">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
          <div className="flex flex-col md:flex-row justify-between items-center">
            <div className="mb-8 md:mb-0">
              <h3 className="text-2xl font-bold text-white mb-2">Duragraph</h3>
              <p className="text-[#8B949E] max-w-md">
                Duragraph Core is Apache 2.0 licensed. Duragraph Cloud is enterprise-ready hosting.
              </p>
            </div>
            <div className="flex flex-wrap gap-8">
              <a href="#" className="text-[#8B949E] hover:text-[#C9D1D9] transition-colors">
                Docs
              </a>
              <a href="#" className="text-[#8B949E] hover:text-[#C9D1D9] transition-colors">
                GitHub
              </a>
              <a href="#" className="text-[#8B949E] hover:text-[#C9D1D9] transition-colors">
                Cloud
              </a>
              <a href="#" className="text-[#8B949E] hover:text-[#C9D1D9] transition-colors">
                Blog
              </a>
            </div>
          </div>
        </div>
      </footer>
    </div>
  )
}
