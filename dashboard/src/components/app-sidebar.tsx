import * as React from "react"
import { Link, useRouterState } from "@tanstack/react-router"
import {
  Bot,
  Gauge,
  LayoutDashboard,
  MessageSquare,
  Play,
  Search,
  Sparkles,
  Users,
  type LucideIcon,
} from "lucide-react"

import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
  SidebarSeparator,
} from "@/components/ui/sidebar"
import { NavTheme } from "@/components/nav-theme"
import { NavUser } from "@/components/nav-user"
import { useCapabilities } from "@/hooks/useCapabilities"
import { getUser } from "@/lib/auth"

// Composed from shadcn's sidebar-07 block primitives:
//   * SidebarHeader  — brand / control-plane row (no team switcher; we
//     have a single workspace today)
//   * SidebarContent — three navigable sections + admin (gated on
//     multitenant capability)
//   * SidebarFooter  — <NavUser> with the real authenticated user
//
// Configuration links (Profile, Settings, API keys) live inside
// <NavUser>'s dropdown rather than as a top-level sidebar group —
// matches the sidebar-07 reference and reduces the always-visible
// surface to actual navigation.

interface NavItem {
  title: string
  to: string
  icon: LucideIcon
}

interface NavGroup {
  label: string
  items: NavItem[]
}

const workspaceGroup: NavGroup = {
  label: "Workspace",
  items: [
    { title: "Dashboard", to: "/", icon: LayoutDashboard },
    { title: "Runs", to: "/runs", icon: Play },
    { title: "Assistants", to: "/assistants", icon: Bot },
    { title: "Threads", to: "/threads", icon: MessageSquare },
  ],
}

// Builder, Deployments, Analytics and Costs were sidebar entries
// that pointed at unimplemented backends (mock data / local-only
// state). Removed until the engine surfaces real endpoints — see
// duragraph-spec/roadmap.yml for the planned milestones.
const playgroundGroup: NavGroup = {
  label: "Playground",
  items: [{ title: "Playground", to: "/playground", icon: Sparkles }],
}

const observabilityGroup: NavGroup = {
  label: "Observability",
  items: [{ title: "Traces", to: "/traces", icon: Search }],
}

// Admin group renders only when the engine reports multitenant
// capability. Same gate the legacy sidebar used.
const adminGroup: NavGroup = {
  label: "Admin",
  items: [
    { title: "Users", to: "/admin/users", icon: Users },
    { title: "Metrics", to: "/admin/metrics", icon: Gauge },
  ],
}

function NavGroupSection({
  group,
  currentPath,
}: {
  group: NavGroup
  currentPath: string
}) {
  return (
    <SidebarGroup>
      <SidebarGroupLabel>{group.label}</SidebarGroupLabel>
      <SidebarGroupContent>
        <SidebarMenu>
          {group.items.map((item) => {
            const isActive =
              item.to === "/"
                ? currentPath === "/"
                : currentPath === item.to ||
                  currentPath.startsWith(item.to + "/")
            return (
              <SidebarMenuItem key={item.to}>
                <SidebarMenuButton asChild isActive={isActive} tooltip={item.title}>
                  <Link to={item.to}>
                    <item.icon />
                    <span>{item.title}</span>
                  </Link>
                </SidebarMenuButton>
              </SidebarMenuItem>
            )
          })}
        </SidebarMenu>
      </SidebarGroupContent>
    </SidebarGroup>
  )
}

export function AppSidebar(props: React.ComponentProps<typeof Sidebar>) {
  const router = useRouterState()
  const currentPath = router.location.pathname
  const capabilities = useCapabilities()
  const user = getUser()

  const groups: NavGroup[] = [
    workspaceGroup,
    playgroundGroup,
    observabilityGroup,
  ]
  if (capabilities?.platformEnabled) {
    groups.push(adminGroup)
  }

  return (
    <Sidebar collapsible="icon" {...props}>
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton size="lg" asChild>
              <Link to="/">
                <div className="flex aspect-square size-8 items-center justify-center overflow-hidden rounded-lg bg-background">
                  <img
                    src="/logo.svg"
                    alt="DuraGraph"
                    className="size-7 object-contain"
                  />
                </div>
                <div className="grid flex-1 text-left text-sm leading-tight">
                  <span className="truncate font-semibold">
                    <span className="text-primary">Dura</span>Graph
                  </span>
                  <span className="truncate text-xs text-muted-foreground">
                    Control Plane
                  </span>
                </div>
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>

      <SidebarContent>
        {groups.map((group) => (
          <NavGroupSection
            key={group.label}
            group={group}
            currentPath={currentPath}
          />
        ))}
      </SidebarContent>

      <SidebarFooter>
        <NavTheme />
        <SidebarSeparator />
        <NavUser
          user={user ? { email: user.email, role: user.role } : null}
        />
      </SidebarFooter>

      <SidebarRail />
    </Sidebar>
  )
}
