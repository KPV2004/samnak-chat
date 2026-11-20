import type { Message } from "@samnak/core";
import type { ComponentType } from "react";
import { FallbackCard } from "../pages/module-ui";
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
  const Known = cardRenderers[message.kind];
  if (Known) return <Known message={message} />;
  return <FallbackCard message={message} />;
}
