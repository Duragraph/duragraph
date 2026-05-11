import { Link, useRouterState } from "@tanstack/react-router"
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb"
import { Separator } from "@/components/ui/separator"
import { SidebarTrigger } from "@/components/ui/sidebar"

// Topbar — the page header that sits at the top of <SidebarInset>.
// Hosts only the sidebar collapse trigger + a URL-derived breadcrumb.
// Theme switching and user identity have moved into the sidebar
// (NavTheme / NavUser) so the bar stays focused on navigation.

export function Topbar() {
  const router = useRouterState()
  const pathSegments = router.location.pathname.split("/").filter(Boolean)

  return (
    <header className="sticky top-0 z-10 flex h-14 shrink-0 items-center gap-2 border-b bg-background px-4">
      <SidebarTrigger className="-ml-1" />
      <Separator orientation="vertical" className="mr-2 h-4" />

      <Breadcrumb className="flex-1">
        <BreadcrumbList>
          <BreadcrumbItem className="hidden md:block">
            <BreadcrumbLink asChild>
              <Link to="/">Home</Link>
            </BreadcrumbLink>
          </BreadcrumbItem>
          {pathSegments.map((segment, index) => {
            const isLast = index === pathSegments.length - 1
            const href = "/" + pathSegments.slice(0, index + 1).join("/")
            const label =
              segment.charAt(0).toUpperCase() +
              segment.slice(1).replace(/-/g, " ")

            return (
              <BreadcrumbItem key={segment}>
                <BreadcrumbSeparator className="hidden md:block" />
                {isLast ? (
                  <BreadcrumbPage>{label}</BreadcrumbPage>
                ) : (
                  <BreadcrumbLink asChild>
                    <Link to={href}>{label}</Link>
                  </BreadcrumbLink>
                )}
              </BreadcrumbItem>
            )
          })}
        </BreadcrumbList>
      </Breadcrumb>
    </header>
  )
}
