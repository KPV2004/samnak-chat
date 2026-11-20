import type { ReactNode } from "react";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

interface Props extends Omit<React.ComponentProps<typeof Button>, "children" | "aria-label"> {
  /** The accessible name, and the tooltip. */
  label: string;
  expanded?: boolean;
  children: ReactNode;
}

/** An icon button of the message toolbar and the composer: a ghost `Button` whose label is its name and its tooltip. */
export function IconTool({ label, expanded, className, children, ...props }: Props) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          aria-label={label}
          aria-expanded={expanded}
          className={cn("text-muted-foreground hover:text-foreground", className)}
          {...props}
        >
          {children}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}
