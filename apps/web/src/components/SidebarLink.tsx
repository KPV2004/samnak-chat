import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";
import { Link } from "react-router";
import { cn } from "@/lib/utils";
import { navItemClass } from "./NavItem";

/**
 * A row of an app sidebar that points at a view: a path with a query string. The router's NavLink
 * only compares paths, so the caller says whether the view is the current one (`current`), and
 * the row takes the same look as NavItem.
 */
export function SidebarLink({
  to,
  icon: Icon,
  current,
  trailing,
  children,
}: {
  to: string;
  icon?: LucideIcon;
  current: boolean;
  trailing?: ReactNode;
  children: ReactNode;
}) {
  return (
    <Link to={to} aria-current={current ? "page" : undefined} className={cn(navItemClass)}>
      {Icon && <Icon aria-hidden="true" className="size-4 shrink-0 opacity-80" />}
      <span className="min-w-0 flex-1 truncate">{children}</span>
      {trailing}
    </Link>
  );
}

/** A small count on a sidebar row; nothing while it is zero. */
export function CountBadge({ n, label, capped }: { n: number; label: string; capped?: boolean }) {
  if (n <= 0) return null;
  return (
    <span className="grid h-5 min-w-5 shrink-0 place-items-center rounded-full bg-current/15 px-1.5 text-[11px] font-medium text-sidebar-foreground in-aria-[current=page]:text-inherit">
      <span aria-hidden="true">
        {n}
        {capped ? "+" : ""}
      </span>
      <span className="sr-only">{label}</span>
    </span>
  );
}

/** The button at the head of an app sidebar ("New request", "New ticket"). */
export function SidebarAction({ to, icon: Icon, children }: { to: string; icon: LucideIcon; children: ReactNode }) {
  return (
    <Link
      to={to}
      className="flex h-9 items-center justify-center gap-2 rounded-md bg-sidebar-primary px-3 text-sm font-medium text-sidebar-primary-foreground outline-none transition-[background-color,scale] duration-150 hover:bg-sidebar-primary/90 focus-visible:ring-2 focus-visible:ring-sidebar-ring focus-visible:ring-offset-2 focus-visible:ring-offset-sidebar active:bg-sidebar-primary/80 motion-safe:active:scale-[0.97]"
    >
      <Icon aria-hidden="true" className="size-4" />
      {children}
    </Link>
  );
}
