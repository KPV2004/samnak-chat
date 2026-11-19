import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";
import { NavLink } from "react-router";
import { cn } from "@/lib/utils";

/** A row of the dark sidebar: it tints on hover, dims on press, and fills with the mint of `sidebar-primary` while it is the current page. */
export const navItemClass = cn(
  "flex h-8 items-center gap-2 rounded-md px-2 text-sm text-muted-foreground outline-none transition-colors duration-150 md:h-7",
  "not-aria-[current=page]:hover:bg-sidebar-accent not-aria-[current=page]:hover:text-sidebar-accent-foreground",
  "not-aria-[current=page]:active:bg-sidebar-accent/60",
  "focus-visible:ring-2 focus-visible:ring-sidebar-ring",
  "aria-[current=page]:bg-sidebar-primary aria-[current=page]:font-medium aria-[current=page]:text-sidebar-primary-foreground",
);

/** A link in the sidebar. The router marks the current page with aria-current, which the styles read. */
export function NavItem({
  to,
  icon: Icon,
  end,
  children,
}: {
  to: string;
  icon: LucideIcon;
  end?: boolean;
  children: ReactNode;
}) {
  return (
    <NavLink to={to} end={end} className={navItemClass}>
      <Icon aria-hidden="true" className="size-4 shrink-0 opacity-80" />
      <span className="truncate">{children}</span>
    </NavLink>
  );
}
