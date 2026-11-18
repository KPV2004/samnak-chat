import type { Message } from "@samnak/core";
import type { ComponentType } from "react";
import { useT } from "../i18n/useT";
import { TaskCard } from "../task/TaskCard";

/**
 * Renderers for the card kinds this client knows, by `kind`. The business modules
 * (approvals, tickets) add theirs here; until then every card shows its fallback.
 */
export const cardRenderers: Record<string, ComponentType<{ message: Message }>> = {
  "task.item": TaskCard,
};

/** A card message: its own renderer when the kind is known, otherwise `fallback_text` (PLT-11). */
export function CardBody({ message }: { message: Message }) {
  const t = useT();
  const Known = cardRenderers[message.kind];
  if (Known) return <Known message={message} />;
  return (
    <div className="grid gap-1 rounded-lg border bg-card px-3 py-2 text-sm text-card-foreground">
      <small className="text-xs text-muted-foreground">{t("chat.card")}</small>
      <div>{message.fallback_text || message.body}</div>
    </div>
  );
}
