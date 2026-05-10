import { Link, useNavigate } from "@tanstack/react-router"
import { Moon, Sun, Monitor, User, Settings, LogOut, Key, Menu } from "lucide-react"
import { clearAuth, getUser } from "@/lib/auth"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { useThemeStore } from "@/stores/theme"
import { useUIStore } from "@/stores/ui"
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb"
import { useRouterState } from "@tanstack/react-router"

export function Topbar() {
  const { theme, setTheme } = useThemeStore()
  const { toggleMobileMenu } = useUIStore()
  const router = useRouterState()
  const navigate = useNavigate()
  const pathSegments = router.location.pathname.split("/").filter(Boolean)

  // Pull from localStorage at render time. We don't subscribe to changes
  // because the only mutation is sign-in / sign-out, both of which
  // trigger a navigate that re-renders this component.
  const authUser = getUser()
  const displayUser = authUser ?? {
    email: "unknown@example.com",
    role: "user",
  }

  const handleLogout = () => {
    clearAuth()
    navigate({ to: "/login" })
  }

  const getInitials = (email: string) => {
    // Fallback initials from email local-part: "alice@example.com" → "AL"
    const local = email.split("@")[0] ?? "?"
    return local.slice(0, 2).toUpperCase()
  }

  return (
    <header className="flex items-center justify-between h-14 px-4 md:px-6 bg-card shadow-[var(--shadow-card)]">
      {/* Mobile Menu Toggle */}
      <Button
        variant="ghost"
        size="icon"
        className="h-8 w-8 md:hidden"
        onClick={toggleMobileMenu}
      >
        <Menu className="h-5 w-5" />
        <span className="sr-only">Toggle menu</span>
      </Button>

      {/* Breadcrumb */}
      <Breadcrumb className="hidden md:flex">
        <BreadcrumbList className="text-sm">
          <BreadcrumbItem>
            <BreadcrumbLink href="/">Home</BreadcrumbLink>
          </BreadcrumbItem>
          {pathSegments.map((segment, index) => {
            const isLast = index === pathSegments.length - 1
            const href = "/" + pathSegments.slice(0, index + 1).join("/")
            const label = segment.charAt(0).toUpperCase() + segment.slice(1).replace(/-/g, ' ')

            return (
              <BreadcrumbItem key={segment}>
                <BreadcrumbSeparator />
                {isLast ? (
                  <BreadcrumbPage>{label}</BreadcrumbPage>
                ) : (
                  <BreadcrumbLink href={href}>{label}</BreadcrumbLink>
                )}
              </BreadcrumbItem>
            )
          })}
        </BreadcrumbList>
      </Breadcrumb>

      {/* Actions */}
      <div className="flex items-center gap-2">
        {/* Theme Toggle */}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon" className="h-8 w-8">
              {theme === 'system' ? (
                <Monitor className="h-4 w-4" />
              ) : theme === 'dark' ? (
                <Moon className="h-4 w-4" />
              ) : (
                <Sun className="h-4 w-4" />
              )}
              <span className="sr-only">Toggle theme</span>
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onClick={() => setTheme("light")}>
              <Sun className="h-4 w-4 mr-2" />
              Light
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => setTheme("dark")}>
              <Moon className="h-4 w-4 mr-2" />
              Dark
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => setTheme("system")}>
              <Monitor className="h-4 w-4 mr-2" />
              System
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>

        {/* User Menu */}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" className="h-8 px-2 gap-2">
              <Avatar className="h-7 w-7">
                <AvatarImage src="" alt={displayUser.email} />
                <AvatarFallback className="text-xs bg-primary/10 text-primary">
                  {getInitials(displayUser.email)}
                </AvatarFallback>
              </Avatar>
              <span className="text-sm font-medium hidden md:inline-block">
                {displayUser.email}
              </span>
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-56">
            <DropdownMenuLabel>
              <div className="flex flex-col space-y-1">
                <p className="text-sm font-medium">{displayUser.email}</p>
                <p className="text-xs text-muted-foreground capitalize">
                  {displayUser.role}
                </p>
              </div>
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem asChild>
              <Link to="/profile" className="cursor-pointer">
                <User className="h-4 w-4 mr-2" />
                Profile
              </Link>
            </DropdownMenuItem>
            <DropdownMenuItem asChild>
              <Link to="/settings" className="cursor-pointer">
                <Settings className="h-4 w-4 mr-2" />
                Settings
              </Link>
            </DropdownMenuItem>
            <DropdownMenuItem asChild>
              <Link to="/settings" search={{ tab: "api-keys" }} className="cursor-pointer">
                <Key className="h-4 w-4 mr-2" />
                API Keys
              </Link>
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              onSelect={handleLogout}
              className="text-destructive focus:text-destructive cursor-pointer"
            >
              <LogOut className="h-4 w-4 mr-2" />
              Log out
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </header>
  )
}
