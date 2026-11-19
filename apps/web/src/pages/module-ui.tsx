import type { Message } from "@samnak/core";
import { ArrowLeft } from "lucide-react";
import { useId, type ComponentProps, type ReactNode } from "react";
import { Link } from "react-router";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Table } from "@/components/ui/table";
import { cn } from "@/lib/utils";
import { AccountLoading } from "../components/AccountStates";
import { useT } from "../i18n/useT";

/*
 * The parts the module pages (tasks, approvals, helpdesk, groups) and the settings pages share, so that
 * a list, a table, a panel, a filter row and a detail page look the same everywhere. Page header,
 * empty state and load error come from components/ (`Page`, `EmptyState`, `AccountLoadError`).
 */

/** "Back to the list" in the header's action slot of a detail page. */
export function BackLink({ to, children }: { to: string; children: ReactNode }) {
  return (
    <Button asChild variant="ghost" size="sm">
      <Link to={to}>
        <ArrowLeft />
        {children}
      </Link>
    </Button>
  );
}

/** The filter row above a list: labelled controls side by side, wrapping on a phone. */
export function Toolbar({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div role="group" aria-label={label} className="flex flex-wrap items-center gap-x-4 gap-y-2">
      {children}
    </div>
  );
}

/** One control of a `Toolbar` with its label in front. `children` receives the id to put on the control. */
export function ToolbarField({ label, children }: { label: string; children: (id: string) => ReactNode }) {
  const id = useId();
  return (
    <div className="flex items-center gap-2">
      <Label htmlFor={id} className="text-muted-foreground">
        {label}
      </Label>
      {children(id)}
    </div>
  );
}

/** A titled block: the one card of the module pages (white, `rounded-lg`, 1px border, no shadow). */
export function Panel({
  title,
  actions,
  footer,
  className,
  bodyClassName,
  children,
  ...rest
}: Omit<ComponentProps<"section">, "title"> & {
  title?: ReactNode;
  actions?: ReactNode;
  footer?: ReactNode;
  bodyClassName?: string;
}) {
  return (
    <section data-slot="card" className={cn("rounded-lg border bg-card text-card-foreground", className)} {...rest}>
      {(title || actions) && (
        <header className="flex flex-wrap items-center justify-between gap-2 border-b px-4 py-3">
          {title && <h2 className="text-base font-semibold">{title}</h2>}
          {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
        </header>
      )}
      <div className={cn("grid gap-4 p-4", bodyClassName)}>{children}</div>
      {footer && <footer className="flex flex-wrap items-center justify-end gap-2 border-t px-4 py-3">{footer}</footer>}
    </section>
  );
}

/** The one list style: rows divided by hairlines inside a bordered, rounded box. */
export function DataList({ className, ...props }: ComponentProps<"ul">) {
  return (
    <ul className={cn("m-0 list-none divide-y overflow-hidden rounded-lg border bg-card p-0", className)} {...props} />
  );
}

/**
 * A row of a `DataList`. A row that leads somewhere or selects something is `interactive`: it tints on
 * hover and a step more on press. Put `stretchedLink` on its main link or button so that the whole row
 * is the target; other controls in the row then need `relative` to stay clickable.
 */
export function DataRow({
  interactive,
  selected,
  className,
  ...props
}: ComponentProps<"li"> & { interactive?: boolean; selected?: boolean }) {
  return (
    <li
      className={cn(
        "relative flex flex-wrap items-center justify-between gap-x-4 gap-y-1 px-4 py-3",
        interactive && "transition-colors hover:bg-muted/60 has-[:active]:bg-muted",
        selected && "bg-accent",
        className,
      )}
      {...props}
    />
  );
}

/** Classes that stretch a link or button over its whole `DataRow`. */
export const stretchedLink = "after:absolute after:inset-0 after:content-['']";

/** The one table style: a bordered box with a quiet header row. */
export function DataTable({ className, ...props }: ComponentProps<typeof Table>) {
  return (
    <div className="overflow-hidden rounded-lg border bg-card">
      <Table
        className={cn(
          "[&_td]:px-4 [&_td]:py-3 [&_th]:px-4 [&_thead_th]:bg-muted/40 [&_thead_th]:text-xs [&_thead_th]:text-muted-foreground [&_tbody_th]:py-3",
          className,
        )}
        {...props}
      />
    </div>
  );
}

/** Placeholder rows shaped like a `DataList`, announced as "Loading…". */
export function ListLoading({ rows = 3 }: { rows?: number }) {
  return (
    <AccountLoading>
      <div className="divide-y overflow-hidden rounded-lg border bg-card">
        {Array.from({ length: rows }, (_, i) => (
          <div key={i} className="grid gap-2 px-4 py-3">
            <Skeleton className="h-4 w-48 max-w-full" />
            <Skeleton className="h-3 w-32 max-w-full" />
          </div>
        ))}
      </div>
    </AccountLoading>
  );
}

/** Placeholder for a detail page: a block for the content and one for the properties. */
export function DetailLoading() {
  return (
    <AccountLoading className="lg:grid-cols-[minmax(0,1fr)_20rem] lg:items-start lg:gap-6">
      <div className="grid gap-4">
        <Skeleton className="h-40 w-full rounded-lg" />
        <Skeleton className="h-32 w-full rounded-lg" />
      </div>
      <Skeleton className="h-48 w-full rounded-lg" />
    </AccountLoading>
  );
}

/** "Load more" under a list that has another page. */
export function LoadMore({ loading, onClick }: { loading: boolean; onClick: () => void }) {
  const t = useT();
  return (
    <div className="flex justify-center">
      <Button type="button" variant="outline" size="sm" disabled={loading} onClick={onClick}>
        {t("loadMore")}
      </Button>
    </div>
  );
}

/**
 * A detail page: the content on the left and a properties panel on the right from `lg`. Below it the
 * properties come first (they are short) and the content follows.
 */
export function DetailLayout({
  aside,
  asideLabel,
  children,
}: {
  aside: ReactNode;
  asideLabel: string;
  children: ReactNode;
}) {
  return (
    <div className="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_20rem]">
      <div className="grid min-w-0 gap-6">{children}</div>
      <aside aria-label={asideLabel} className="order-first grid min-w-0 gap-6 lg:order-none">
        {aside}
      </aside>
    </div>
  );
}

/** Label and value pairs of a properties panel. */
export function Properties({ children }: { children: ReactNode }) {
  return <dl className="m-0 grid gap-3">{children}</dl>;
}

export function Property({ label, children, multiline }: { label: string; children: ReactNode; multiline?: boolean }) {
  return (
    <div className="grid grid-cols-[6.5rem_minmax(0,1fr)] items-baseline gap-x-3">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className={cn("m-0 text-sm break-words", multiline && "whitespace-pre-wrap")}>{children}</dd>
    </div>
  );
}

/**
 * The one card of a chat message (task, approval, helpdesk ticket and the fallback): a status bar on the
 * left, the kind and the status badge on top, the title, a line of facts, then the actions under a hairline.
 * `bar` is a `border-l-*` class from the module's tones; `tone` is only the status.
 */
export function CardFrame({
  kind,
  badge,
  bar,
  title,
  meta,
  actions,
  children,
  className,
}: {
  kind: ReactNode;
  badge?: ReactNode;
  bar?: string;
  title?: ReactNode;
  meta?: ReactNode;
  actions?: ReactNode;
  children?: ReactNode;
  className?: string;
}) {
  return (
    <article
      className={cn(
        "grid max-w-md gap-2 rounded-lg border border-l-[3px] bg-card p-3 text-sm text-card-foreground",
        bar ?? "border-l-border",
        className,
      )}
    >
      <div className="flex items-start justify-between gap-2">
        <p className="m-0 text-xs text-muted-foreground">{kind}</p>
        {badge}
      </div>
      {(title || meta) && (
        <div className="grid gap-0.5">
          {title && <p className="m-0 font-medium break-words">{title}</p>}
          {meta && <div className="text-xs text-muted-foreground">{meta}</div>}
        </div>
      )}
      {children}
      {actions && <div className="flex flex-wrap items-center gap-2 border-t pt-2">{actions}</div>}
    </article>
  );
}

/** The card of a kind this client does not know, or of a module that is off: its `fallback_text` in the card frame (PLT-11). */
export function FallbackCard({ message }: { message: Message }) {
  const t = useT();
  return (
    <CardFrame
      kind={t("chat.card")}
      title={<span className="font-normal">{message.fallback_text || message.body}</span>}
    />
  );
}
