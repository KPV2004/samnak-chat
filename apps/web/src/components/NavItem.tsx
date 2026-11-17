import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";
import { NavLink } from "react-router";
import { cn } from "@/lib/utils";

export const navItemClass = cn(
  "flex h-8 items-center gap-2 rounded-md px-2 text-sm text-muted-foreground outline-none transition-colors",
  "hover:bg-sidebar-accent hover:text-sidebar-accent-foreground",
  "focus-visible:ring-[3px] focus-visible:ring-sidebar-ring/50",
  "aria-[current=page]:bg-sidebar-accent aria-[current=page]:font-medium aria-[current=page]:text-sidebar-accent-foreground",
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
      <Icon className="size-4 shrink-0" />
      <span className="truncate">{children}</span>
    </NavLink>
  );
}
