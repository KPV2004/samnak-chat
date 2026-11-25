import { ChevronDown } from "lucide-react";
import type { ReactNode } from "react";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { useT } from "@/i18n/useT";

/** The optional half of a form, collapsed by default so the main form stays short. */
export function MoreOptions({ children }: { children: ReactNode }) {
  const t = useT();
  return (
    <Collapsible>
      <CollapsibleTrigger className="group flex items-center gap-1 rounded-md text-sm font-medium text-primary outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring">
        <ChevronDown aria-hidden="true" className="size-4 transition-transform duration-150 group-data-[state=open]:rotate-180" />
        {t("form.more")}
      </CollapsibleTrigger>
      <CollapsibleContent className="grid gap-4 pt-3">{children}</CollapsibleContent>
    </Collapsible>
  );
}
