import type { Workspace } from "@samnak/core";
import { Plus } from "lucide-react";
import { Link } from "react-router";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { useT } from "@/i18n/useT";
import { cn } from "@/lib/utils";
import { useWorkspaces } from "../state/queries";
import { useSession } from "../state/session";
import { initialOf } from "./UserAvatar";

/** The two characters a workspace tile shows: the first letters of its first two words, else its first two letters. */
export function workspaceInitials(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean);
  if (words.length >= 2) return initialOf(words[0]!) + initialOf(words[1]!);
  const letters = Array.from(name.trim()).slice(0, 2).join("");
  return letters.toLocaleUpperCase() || "?";
}

const tileClass = cn(
  "relative grid size-9 place-items-center rounded-lg text-sm font-semibold outline-none transition-[background-color,scale] duration-150",
  "focus-visible:ring-2 focus-visible:ring-sidebar-ring focus-visible:ring-offset-2 focus-visible:ring-offset-rail",
  "motion-safe:hover:-translate-y-px motion-safe:active:scale-95",
);

/**
 * The narrow dark column on the far left: one tile per workspace (the current one is filled), and
 * a tile to make a new one. Switching only changes the selected workspace, as the old select did.
 */
export function WorkspaceRail({ onNavigate }: { onNavigate?: () => void }) {
  const t = useT();
  const session = useSession();
  const { workspaces, current } = useWorkspaces();

  return (
    <nav aria-label={t("workspace.list")} className="on-rail flex w-14 shrink-0 flex-col items-center gap-2 bg-rail py-2">
      {workspaces?.map((w: Workspace) => {
        const isCurrent = w.id === current?.id;
        return (
          <Tooltip key={w.id}>
            <TooltipTrigger asChild>
              <button
                type="button"
                aria-label={w.name}
                aria-current={isCurrent ? "true" : undefined}
                className={cn(
                  tileClass,
                  isCurrent
                    ? "bg-sidebar-primary text-sidebar-primary-foreground"
                    : "bg-sidebar-foreground/15 text-sidebar-foreground hover:bg-sidebar-foreground/25",
                )}
                onClick={() => {
                  session.setWorkspaceId(w.id);
                  onNavigate?.();
                }}
              >
                {isCurrent && (
                  <span aria-hidden="true" className="absolute top-1/2 -left-2.5 h-5 w-1 -translate-y-1/2 rounded-r-full bg-sidebar-primary" />
                )}
                <span aria-hidden="true">{workspaceInitials(w.name)}</span>
              </button>
            </TooltipTrigger>
            <TooltipContent side="right">{w.name}</TooltipContent>
          </Tooltip>
        );
      })}
      <Tooltip>
        <TooltipTrigger asChild>
          <Link
            to="/workspaces/new"
            aria-label={t("workspace.new")}
            className={cn(tileClass, "border border-dashed border-sidebar-foreground/40 text-sidebar-foreground hover:bg-sidebar-accent")}
            onClick={onNavigate}
          >
            <Plus aria-hidden="true" className="size-4" />
          </Link>
        </TooltipTrigger>
        <TooltipContent side="right">{t("workspace.new")}</TooltipContent>
      </Tooltip>
    </nav>
  );
}
