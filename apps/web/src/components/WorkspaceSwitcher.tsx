import { Check, ChevronsUpDown, Plus } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Link } from "react-router";
import { useT } from "@/i18n/useT";
import { cn } from "@/lib/utils";
import { useWorkspaces } from "../state/queries";
import { useSession } from "../state/session";
import { workspaceInitials } from "./WorkspaceRail";

/** The workspace at the top of the left column: initials, name and a menu of the other workspaces. */
export function WorkspaceSwitcher({ onNavigate }: { onNavigate?: () => void }) {
  const t = useT();
  const session = useSession();
  const { workspaces, current } = useWorkspaces();
  const [open, setOpen] = useState(false);
  const box = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    function onPointer(e: PointerEvent) {
      if (!box.current?.contains(e.target as Node)) setOpen(false);
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") setOpen(false);
    }
    document.addEventListener("pointerdown", onPointer);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("pointerdown", onPointer);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  if (!current) return null;
  return (
    <div ref={box} className="relative">
      <button
        type="button"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label={t("workspace.switch", { name: current.name })}
        onClick={() => setOpen((v) => !v)}
        className={cn(
          "flex h-11 w-full items-center gap-2 rounded-md px-1.5 text-left outline-none transition-colors duration-150",
          "hover:bg-sidebar-accent active:bg-sidebar-accent/60 focus-visible:ring-2 focus-visible:ring-sidebar-ring",
        )}
      >
        <span
          aria-hidden="true"
          className="grid size-8 shrink-0 place-items-center rounded-lg bg-primary text-sm font-semibold text-primary-foreground"
        >
          {workspaceInitials(current.name)}
        </span>
        <span className="hidden min-w-0 flex-1 truncate text-sm font-semibold lg:block">{current.name}</span>
        <ChevronsUpDown aria-hidden="true" className="hidden size-4 shrink-0 text-muted-foreground lg:block" />
      </button>
      {open && (
        <div
          role="menu"
          aria-label={t("workspace.list")}
          className="absolute top-full left-0 z-(--z-dropdown) mt-1 grid w-60 gap-0.5 rounded-md border bg-popover p-1 text-popover-foreground shadow-md"
        >
          <p className="m-0 px-2 py-1.5 text-xs font-semibold text-muted-foreground">{t("workspace.list")}</p>
          {workspaces?.map((w) => (
            <button
              key={w.id}
              type="button"
              role="menuitem"
              onClick={() => {
                session.setWorkspaceId(w.id);
                setOpen(false);
                onNavigate?.();
              }}
              className="flex h-9 items-center gap-2 rounded-sm px-2 text-left text-sm outline-none hover:bg-accent focus-visible:bg-accent"
            >
              <span className="min-w-0 flex-1 truncate">{w.name}</span>
              {w.id === current.id && <Check aria-hidden="true" className="size-4" />}
            </button>
          ))}
          <div className="my-1 h-px bg-border" />
          <Link
            to="/workspaces/new"
            role="menuitem"
            onClick={() => {
              setOpen(false);
              onNavigate?.();
            }}
            className="flex h-9 items-center gap-2 rounded-sm px-2 text-sm outline-none hover:bg-accent focus-visible:bg-accent"
          >
            <Plus aria-hidden="true" className="size-4" />
            {t("workspace.new")}
          </Link>
        </div>
      )}
    </div>
  );
}
