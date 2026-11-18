import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

const widths = { narrow: "max-w-xl", default: "max-w-4xl", wide: "max-w-6xl" } as const;

/**
 * The frame of a redesigned page: a centred column, the page title and room for page-level actions.
 * It sets `data-page`, which drops the bottom margin of the fields and errors inside it and tells the
 * shell not to pad the page twice. Use it as the page's root element and build the rest from components/ui.
 */
export function Page({
  title,
  description,
  actions,
  width = "default",
  className,
  children,
}: {
  title: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  width?: keyof typeof widths;
  className?: string;
  children: ReactNode;
}) {
  return (
    <div data-page className={cn("mx-auto grid w-full gap-6 p-4 md:p-6", widths[width], className)}>
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div className="grid gap-1">
          <h1 className="text-xl font-semibold">{title}</h1>
          {description && <p className="text-sm text-muted-foreground">{description}</p>}
        </div>
        {actions && <div className="flex items-center gap-2">{actions}</div>}
      </header>
      {children}
    </div>
  );
}
