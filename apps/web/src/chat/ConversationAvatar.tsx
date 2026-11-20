import type { Channel } from "@samnak/core";
import { Hash, Lock } from "lucide-react";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { cn } from "@/lib/utils";
import { initialOf } from "../components/UserAvatar";
import { useT } from "../i18n/useT";
import { useChat } from "../state/chat";
import { avatarColorFor } from "./avatarColor";
import { useAvatarOf, useNameOf } from "./names";

/** `rail` sits on the dark sidebar; `page` on the page surface. It only decides the colour of the rings that cut one picture out of the next. */
export type AvatarTone = "rail" | "page";

const ring: Record<AvatarTone, string> = { rail: "ring-sidebar", page: "ring-background" };
const badgeTone: Record<AvatarTone, string> = {
  rail: "bg-sidebar text-sidebar-foreground ring-sidebar",
  page: "bg-card text-card-foreground ring-background",
};

/** A round picture of a person: their photo when there is one, else their initial on a colour their id always gets. */
export function PersonAvatar({ id, name, src = "", className }: { id: string; name: string; src?: string; className?: string }) {
  return (
    <Avatar className={cn("size-10", className)}>
      {src && <AvatarImage src={src} alt="" />}
      <AvatarFallback className={cn("text-sm font-medium text-white", avatarColorFor(id))}>
        <span aria-hidden="true">{initialOf(name)}</span>
      </AvatarFallback>
    </Avatar>
  );
}

/**
 * The picture of a conversation, as in Lark: a 1:1 message is the other person's picture, a group
 * message two stacked pictures (and how many more), a channel a rounded tile with its first letter
 * and a `#` or lock badge. It is decoration: the title next to it names the conversation.
 */
export function ConversationAvatar({
  channel,
  title,
  tone = "rail",
  className,
}: {
  channel: Channel;
  title: string;
  tone?: AvatarTone;
  className?: string;
}) {
  const t = useT();
  const me = useChat()?.userId ?? "";
  const nameOf = useNameOf();
  const avatarOf = useAvatarOf();
  const box = cn("relative inline-block size-10 shrink-0", className);

  if (channel.type !== "dm") {
    const BadgeIcon = channel.type === "private" ? Lock : Hash;
    return (
      <span aria-hidden="true" className={box}>
        <span
          className={cn(
            "grid size-full place-items-center rounded-lg text-base font-semibold text-white",
            avatarColorFor(channel.id),
          )}
        >
          {initialOf(title)}
        </span>
        <span className={cn("absolute -right-1 -bottom-1 grid size-4 place-items-center rounded-full ring-2", badgeTone[tone])}>
          <BadgeIcon className="size-2.5" strokeWidth={2.5} />
        </span>
      </span>
    );
  }

  const others = channel.dm_user_ids.filter((id) => id !== me);
  if (others.length <= 1) {
    const id = others[0] ?? me;
    return (
      <span aria-hidden="true" className={box}>
        <PersonAvatar id={id} name={nameOf(id)} src={avatarOf(id)} className="size-full" />
      </span>
    );
  }

  const [first, second] = others as [string, string];
  const more = others.length - 2;
  return (
    <span aria-hidden="true" className={box}>
      <PersonAvatar id={first} name={nameOf(first)} src={avatarOf(first)} className="absolute top-0 left-0 size-[26px] *:data-[slot=avatar-fallback]:text-xs" />
      <PersonAvatar
        id={second}
        name={nameOf(second)}
        src={avatarOf(second)}
        className={cn("absolute right-0 bottom-0 size-[26px] ring-2 *:data-[slot=avatar-fallback]:text-xs", ring[tone])}
      />
      {more > 0 && (
        <span
          title={t("chat.group.more", { n: more })}
          className={cn(
            "absolute -top-1 -right-1.5 grid h-4 min-w-4 place-items-center rounded-full px-1 text-[10px] leading-none font-semibold ring-2",
            badgeTone[tone],
          )}
        >
          +{more}
        </span>
      )}
    </span>
  );
}
