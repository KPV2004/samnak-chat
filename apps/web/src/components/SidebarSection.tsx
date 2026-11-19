import { ChevronRight } from "lucide-react";
import { useState, type ReactNode } from "react";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";

const storageKey = (id: string) => `samnak.sidebar.${id}`;

function readOpen(id: string): boolean {
  try {
    return localStorage.getItem(storageKey(id)) !== "closed";
  } catch {
    return true;
  }
}

/**
 * A sidebar section with a heading that folds it away, as in Slack. Open or closed is remembered per
 * `id` in localStorage. While closed, `badge` (an unread count, say) stays visible on the heading.
 */
export function SidebarSection({
  id,
  title,
  badge,
  children,
}: {
  id: string;
  title: string;
  badge?: ReactNode;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(() => readOpen(id));

  function change(next: boolean) {
    setOpen(next);
    try {
      localStorage.setItem(storageKey(id), next ? "open" : "closed");
    } catch {
      // Storage can be blocked; the section still folds for this visit.
    }
  }

  return (
    <Collapsible open={open} onOpenChange={change} asChild>
      <section>
        <h2 className="m-0 text-xs font-medium">
          <CollapsibleTrigger className="group flex h-7 w-full items-center gap-1 rounded-md px-2 text-left text-muted-foreground outline-none transition-colors duration-150 hover:bg-sidebar-accent hover:text-sidebar-accent-foreground active:bg-sidebar-accent/60 focus-visible:ring-2 focus-visible:ring-sidebar-ring">
            <ChevronRight
              aria-hidden="true"
              className="size-3.5 shrink-0 transition-transform duration-150 group-data-[state=open]:rotate-90"
            />
            <span className="min-w-0 flex-1 truncate">{title}</span>
            {!open && badge}
          </CollapsibleTrigger>
        </h2>
        <CollapsibleContent className="grid gap-0.5 pt-0.5">{children}</CollapsibleContent>
      </section>
    </Collapsible>
  );
}
