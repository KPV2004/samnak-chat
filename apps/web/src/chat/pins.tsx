import { ApiError, type Channel } from "@samnak/core";
import { Pin, PinOff } from "lucide-react";
import type { ComponentType, ReactNode } from "react";
import { toast } from "sonner";
import { useT } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { useChat } from "../state/chat";

/** Any Radix menu item (dropdown or context menu), so one set of entries serves both menus of a conversation. */
export type MenuItem = ComponentType<{ onSelect?: (event: Event) => void; disabled?: boolean; children?: ReactNode }>;

/** Pins or unpins a conversation for me; a refusal (30 pins at most) shows as a toast. */
export function usePin(channel: Channel) {
  const t = useT();
  const store = useChat()?.store;
  const pinned = channel.pinned_at !== null;
  function toggle() {
    store?.setPinned(channel.id, !pinned).catch((err: unknown) => {
      toast.error(err instanceof ApiError && err.status === 409 ? t("chat.pin.limit") : t(errorKey(err)));
    });
  }
  return { pinned, toggle };
}

/** The Pin to top or Unpin entry of a conversation's menu. */
export function PinMenuItem({ channel, Item }: { channel: Channel; Item: MenuItem }) {
  const t = useT();
  const { pinned, toggle } = usePin(channel);
  return (
    <Item onSelect={toggle}>
      {pinned ? <PinOff aria-hidden="true" /> : <Pin aria-hidden="true" />}
      {t(pinned ? "chat.unpin" : "chat.pin")}
    </Item>
  );
}
