import { MessagesSquare } from "lucide-react";
import { Link } from "react-router";
import { useT } from "@/i18n/useT";
import { cn } from "@/lib/utils";

/** The product mark and name. It is a link home unless `to` is null. */
export function Brand({ to = "/", className }: { to?: string | null; className?: string }) {
  const t = useT();
  const content = (
    <>
      <span className="grid size-7 place-items-center rounded-md bg-primary text-primary-foreground">
        <MessagesSquare className="size-4" />
      </span>
      <span className="font-semibold">{t("title")}</span>
    </>
  );
  const classes = cn("flex items-center gap-2 text-foreground", className);
  return to === null ? (
    <p className={cn(classes, "m-0")}>{content}</p>
  ) : (
    <Link to={to} className={cn(classes, "rounded-md outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50")}>
      {content}
    </Link>
  );
}
