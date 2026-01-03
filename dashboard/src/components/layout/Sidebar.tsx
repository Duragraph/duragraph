import { useEffect } from "react"
import { Link, useRouterState } from "@tanstack/react-router"
import { cn } from "@/lib/utils"
import { useUIStore } from "@/stores/ui"
import {
  LayoutDashboard,
  Play,
  Bot,
  MessageSquare,
  Settings,
  ChevronLeft,
  ChevronRight,
  Search,
  BarChart3,
  DollarSign,
  User,
  X,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import { Separator } from "@/components/ui/separator"
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip"

const navItems = [
  { icon: LayoutDashboard, label: "Dashboard", path: "/" },
  { icon: Play, label: "Runs", path: "/runs" },
  { icon: Bot, label: "Assistants", path: "/assistants" },
  { icon: MessageSquare, label: "Threads", path: "/threads" },
]

const observabilityItems = [
  { icon: Search, label: "Traces", path: "/traces" },
  { icon: BarChart3, label: "Analytics", path: "/analytics" },
  { icon: DollarSign, label: "Costs", path: "/costs" },
]

const configItems = [
  { icon: User, label: "Profile", path: "/profile" },
  { icon: Settings, label: "Settings", path: "/settings" },
]

export function Sidebar() {
  const { sidebarCollapsed, toggleSidebar, mobileMenuOpen, setMobileMenuOpen } = useUIStore()
  const router = useRouterState()
  const currentPath = router.location.pathname

  // Close mobile menu on route change
  useEffect(() => {
    setMobileMenuOpen(false)
  }, [currentPath, setMobileMenuOpen])

  // Close mobile menu on escape key
  useEffect(() => {
    const handleEscape = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setMobileMenuOpen(false)
      }
    }
    window.addEventListener("keydown", handleEscape)
    return () => window.removeEventListener("keydown", handleEscape)
  }, [setMobileMenuOpen])

  return (
    <TooltipProvider delayDuration={0}>
      {/* Mobile overlay */}
      {mobileMenuOpen && (
        <div
          className="fixed inset-0 bg-black/50 z-40 md:hidden"
          onClick={() => setMobileMenuOpen(false)}
        />
      )}

      <aside
        className={cn(
          "flex flex-col h-screen bg-sidebar shadow-[var(--shadow-card)] transition-all duration-200",
          // Desktop styles
          "hidden md:flex",
          sidebarCollapsed ? "md:w-14" : "md:w-56",
          // Mobile styles - slide in from left
          mobileMenuOpen && "fixed inset-y-0 left-0 z-50 flex w-64"
        )}
      >
        {/* Logo */}
        <div className="flex items-center justify-between h-14 px-4 border-b border-border">
          <Link to="/" className="flex items-center gap-2">
            <img
              src="/logo.svg"
              alt="DuraGraph"
              className="w-7 h-7 flex-shrink-0"
            />
            {(!sidebarCollapsed || mobileMenuOpen) && (
              <span className="font-semibold text-sm text-foreground">
                DuraGraph
              </span>
            )}
          </Link>
          {/* Mobile close button */}
          <Button
            variant="ghost"
            size="icon"
            className="h-8 w-8 md:hidden"
            onClick={() => setMobileMenuOpen(false)}
          >
            <X className="h-5 w-5" />
          </Button>
        </div>

        {/* Navigation */}
        <nav className="flex-1 p-2 space-y-0.5 overflow-y-auto">
          {navItems.map((item) => (
            <NavItem
              key={item.path}
              {...item}
              isActive={currentPath === item.path}
              collapsed={sidebarCollapsed && !mobileMenuOpen}
            />
          ))}

          <Separator className="my-3" />

          <div className={cn("px-3 py-2", sidebarCollapsed && !mobileMenuOpen && "hidden")}>
            <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
              Observability
            </span>
          </div>

          {observabilityItems.map((item) => (
            <NavItem
              key={item.path}
              {...item}
              isActive={currentPath === item.path}
              collapsed={sidebarCollapsed && !mobileMenuOpen}
            />
          ))}

          <Separator className="my-3" />

          <div className={cn("px-3 py-2", sidebarCollapsed && !mobileMenuOpen && "hidden")}>
            <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
              Config
            </span>
          </div>

          {configItems.map((item) => (
            <NavItem
              key={item.path}
              {...item}
              isActive={currentPath === item.path}
              collapsed={sidebarCollapsed && !mobileMenuOpen}
            />
          ))}
        </nav>

        {/* Collapse Toggle - hidden on mobile */}
        <div className="p-2 border-t border-border hidden md:block">
          <Button
            variant="ghost"
            size="sm"
            className="w-full justify-center h-8"
            onClick={toggleSidebar}
          >
            {sidebarCollapsed ? (
              <ChevronRight className="h-4 w-4" />
            ) : (
              <ChevronLeft className="h-4 w-4" />
            )}
          </Button>
        </div>
      </aside>
    </TooltipProvider>
  )
}

interface NavItemProps {
  icon: React.ElementType
  label: string
  path: string
  isActive: boolean
  collapsed: boolean
}

function NavItem({ icon: Icon, label, path, isActive, collapsed }: NavItemProps) {
  const content = (
    <Link
      to={path}
      className={cn(
        "flex items-center gap-2.5 px-3 py-2 text-sm font-medium transition-colors",
        isActive
          ? "bg-primary/10 text-primary"
          : "text-muted-foreground hover:bg-secondary hover:text-foreground",
        collapsed && "justify-center px-2"
      )}
    >
      <Icon className="h-4 w-4 flex-shrink-0" />
      {!collapsed && <span>{label}</span>}
    </Link>
  )

  if (collapsed) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>{content}</TooltipTrigger>
        <TooltipContent side="right" className="font-medium">
          {label}
        </TooltipContent>
      </Tooltip>
    )
  }

  return content
}
