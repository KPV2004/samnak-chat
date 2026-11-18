import type { ReactNode } from "react";
import { Button } from "@/components/ui/button";
import { useT } from "@/i18n/useT";
import { cn } from "@/lib/utils";

/**
 * Stands in for a list, table or form while it loads. `children` draws the shape of what is coming
 * (Skeleton blocks); the status region announces "Loading…" to screen readers.
 */
export function AccountLoading({ className, children }: { className?: string; children: ReactNode }) {
  const t = useT();
  return (
    <div role="status" className={cn("grid gap-3", className)}>
      <span className="sr-only">{t("loading")}</span>
      {children}
    </div>
  );
}

/** A load that failed. It is an alert, so it is read out when it appears; `onRetry` runs the load again. */
export function AccountLoadError({ message, onRetry }: { message: string; onRetry?: () => void }) {
  const t = useT();
  return (
    <div
      role="alert"
      className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-destructive/30 px-4 py-3"
    >
      <p className="m-0 text-sm text-destructive">{message}</p>
      {onRetry && (
        <Button type="button" variant="outline" size="sm" onClick={onRetry}>
          {t("retry")}
        </Button>
      )}
    </div>
  );
}
