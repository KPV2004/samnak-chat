import { useEffect, useRef, useState } from "react";
import { useT } from "../i18n/useT";
import { useChat } from "../state/chat";
import { useNameOf } from "./names";

/** How long a typing event counts; the server repeats it at most every 3 seconds. */
const typingTtlMs = 5000;

/** The users (other than me) who are typing in a channel right now. */
export function useTyping(channelId: string): string[] {
  const realtime = useChat()?.realtime;
  const me = useChat()?.userId;
  const [typing, setTyping] = useState<string[]>([]);
  const timers = useRef(new Map<string, ReturnType<typeof setTimeout>>());

  useEffect(() => {
    if (!realtime) return;
    const pending = timers.current;
    const forget = (userId: string) => {
      clearTimeout(pending.get(userId));
      pending.delete(userId);
      setTyping((list) => list.filter((id) => id !== userId));
    };
    const offTyping = realtime.on("typing", (d) => {
      if (d.channel_id !== channelId || d.user_id === me) return;
      clearTimeout(pending.get(d.user_id));
      pending.set(
        d.user_id,
        setTimeout(() => forget(d.user_id), typingTtlMs),
      );
      setTyping((list) => (list.includes(d.user_id) ? list : [...list, d.user_id]));
    });
    // Someone who sent their message is no longer typing it.
    const offMessage = realtime.on("message.created", (d) => {
      if (d.channel_id === channelId) forget(d.message.sender_id);
    });
    return () => {
      offTyping();
      offMessage();
      pending.forEach(clearTimeout);
      pending.clear();
      setTyping([]);
    };
  }, [realtime, channelId, me]);

  return typing;
}

/** A polite live region, always mounted so a screen reader announces text that appears later. */
export function TypingIndicator({ channelId }: { channelId: string }) {
  const t = useT();
  const nameOf = useNameOf();
  const typing = useTyping(channelId);
  const [a, b] = typing.map(nameOf);
  const text =
    typing.length === 0
      ? ""
      : typing.length === 1
        ? t("chat.typing.one", { a: a! })
        : typing.length === 2
          ? t("chat.typing.two", { a: a!, b: b! })
          : t("chat.typing.many");
  return (
    <p className="chat-typing muted" role="status">
      {text}
    </p>
  );
}
