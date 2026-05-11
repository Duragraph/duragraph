import { Moon, Sun } from "lucide-react"
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar"
import { Switch } from "@/components/ui/switch"
import { useThemeStore } from "@/stores/theme"

// NavTheme renders the theme toggle in the sidebar footer.
//
// Previously this was a tri-state Light/Dark/System dropdown — that
// required two clicks (open → pick) for a high-frequency toggle and
// didn't read as a "switch." The canonical shadcn dark-mode pattern
// (Switch with Sun/Moon flanking) is a single click and visually
// communicates the bi-state better.
//
// We drop the explicit "System" option because:
//   * the underlying useThemeStore still defaults to "system" on
//     first load, so OS preference still wins until the user clicks
//     once;
//   * a binary switch can't express three states without growing
//     into a Segmented control, which is overkill for the footer.
//
// Two presentations gated on sidebar collapse mode:
//   * Expanded — full row with Sun · <Switch> · Moon. The active
//     icon glows in primary; the other dims.
//   * Collapsed (icon-only) — a single SidebarMenuButton with the
//     current mode's icon; clicking toggles.

function useThemeToggle() {
  const { theme, setTheme } = useThemeStore()
  const isDark =
    theme === "dark" ||
    (theme === "system" &&
      typeof window !== "undefined" &&
      window.matchMedia("(prefers-color-scheme: dark)").matches)
  const toggle = () => setTheme(isDark ? "light" : "dark")
  return { isDark, toggle }
}

export function NavTheme() {
  const { isDark, toggle } = useThemeToggle()

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        {/* Expanded layout — flex row with the switch flanked by
            Sun/Moon. Hidden when the rail collapses to icon-only. */}
        <div className="flex items-center justify-between gap-2 rounded-md px-2 py-1.5 group-data-[collapsible=icon]:hidden">
          <div className="flex items-center gap-2 text-sm">
            <Sun
              className={
                "size-4 transition-colors " +
                (isDark ? "text-muted-foreground" : "text-primary")
              }
            />
            <span className="text-muted-foreground">Theme</span>
          </div>
          <div className="flex items-center gap-2">
            <Switch
              checked={isDark}
              onCheckedChange={toggle}
              aria-label="Toggle dark mode"
            />
            <Moon
              className={
                "size-4 transition-colors " +
                (isDark ? "text-primary" : "text-muted-foreground")
              }
            />
          </div>
        </div>

        {/* Collapsed layout — single click target that toggles.
            Hidden when the sidebar is full-width. */}
        <SidebarMenuButton
          onClick={toggle}
          tooltip={isDark ? "Switch to light mode" : "Switch to dark mode"}
          className="hidden group-data-[collapsible=icon]:flex"
        >
          {isDark ? <Moon /> : <Sun />}
          <span>Theme</span>
        </SidebarMenuButton>
      </SidebarMenuItem>
    </SidebarMenu>
  )
}
