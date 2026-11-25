import type { ReactNode } from "react";
import type { LucideIcon } from "lucide-react";
import { Link } from "react-router";
import { Card } from "@/components/ui/card";
import { cn } from "@/lib/utils";

/** A "waiting" card on Home: a count, up to three rows and one link to the full list. */
export function SummaryCard({
  title,
  icon: Icon,
  count,
  more,
  href,
  seeAll,
  empty,
  children,
}: {
  title: string;
  icon: LucideIcon;
  count: number;
  more: boolean;
  href: string;
  seeAll: string;
  empty: string;
  children: ReactNode;
}) {
  return (
    <Card className="gap-0 p-4">
      <div className="flex items-start justify-between gap-3">
        <Link to={href} className="flex min-w-0 items-center gap-2 text-sm font-semibold hover:underline">
          <Icon aria-hidden="true" className="size-5 shrink-0 text-primary" />
          <span className="truncate">{title}</span>
        </Link>
        <span className="shrink-0 text-2xl font-semibold tabular-nums" aria-label={more ? `${count}+` : String(count)}>
          {more ? `${count}+` : count}
        </span>
      </div>
      {count === 0 ? (
        <p className="m-0 mt-2 text-sm text-muted-foreground">{empty}</p>
      ) : (
        <ul className="mt-2 divide-y">{children}</ul>
      )}
      <Link to={href} className="mt-2 text-sm text-primary underline underline-offset-2">
        {seeAll}
      </Link>
    </Card>
  );
}

/** One row of a summary card: the main line, an optional second line and a trailing slot. */
export function SummaryRow({
  to,
  title,
  subtitle,
  trailing,
  className,
}: {
  to?: string;
  title: ReactNode;
  subtitle?: ReactNode;
  trailing?: ReactNode;
  className?: string;
}) {
  const body = (
    <>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-sm font-medium">{title}</span>
        {subtitle && <span className="block truncate text-xs text-muted-foreground">{subtitle}</span>}
      </span>
      {trailing}
    </>
  );
  return (
    <li className={cn("flex items-center gap-2 py-2", className)}>
      {to ? (
        <Link to={to} className="flex min-w-0 flex-1 items-center gap-3 rounded-md hover:bg-accent/60">
          {body}
        </Link>
      ) : (
        <span className="flex min-w-0 flex-1 items-center gap-3">{body}</span>
      )}
    </li>
  );
}
