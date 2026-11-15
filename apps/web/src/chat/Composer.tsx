import { useQuery } from "@tanstack/react-query";
import { useId, useLayoutEffect, useRef, useState } from "react";
import { useT } from "../i18n/useT";
import { useChat } from "../state/chat";
import { MentionList } from "./MentionList";
import { insertMention, mentionAt, type MentionOption } from "./mentions";
import { useNameOf } from "./names";

export const maxLength = 4000;
/** The server fans typing out at most this often, so the client does not send more. */
const typingEveryMs = 3000;
const maxOptions = 6;

interface Props {
  channelId: string;
  /** Sends replies under this top-level message instead of into the channel. */
  threadRootId?: string;
  label?: string;
}

export function Composer({ channelId, threadRootId, label }: Props) {
  const t = useT();
  const chat = useChat();
  const store = chat?.store;
  const nameOf = useNameOf();
  const [text, setText] = useState("");
  const [caret, setCaret] = useState(0);
  const [active, setActive] = useState(0);
  const [dismissed, setDismissed] = useState(false);
  const boxRef = useRef<HTMLTextAreaElement>(null);
  const nextCaret = useRef<number | null>(null);
  const lastTyping = useRef(0);
  const uid = useId();

  const length = [...text].length;
  const tooLong = length > maxLength;
  const canSend = text.trim() !== "" && !tooLong && store !== undefined;

  const members = useQuery({
    queryKey: ["chat-channel-members", channelId],
    enabled: chat !== null,
    staleTime: 30_000,
    queryFn: async () => (await chat!.api.listChannelMembers(channelId)).items,
  });

  const mention = dismissed ? null : mentionAt(text, caret);
  const options: MentionOption[] = [];
  if (mention) {
    const q = mention.query.toLowerCase();
    if ("channel".startsWith(q)) {
      options.push({ token: "<!channel>", label: t("chat.mentionChannel"), detail: t("chat.mention.channel") });
    }
    for (const m of members.data ?? []) {
      const name = m.display_name || nameOf(m.user_id);
      if (m.user_id !== chat?.userId && name.toLowerCase().includes(q)) {
        options.push({ token: `<@${m.user_id}>`, label: name });
      }
    }
  }
  const shown = options.slice(0, maxOptions);
  const open = shown.length > 0;
  const current = Math.min(active, shown.length - 1);

  // Puts the caret after an inserted mention once the new text is in the box.
  useLayoutEffect(() => {
    if (nextCaret.current !== null && boxRef.current) {
      boxRef.current.setSelectionRange(nextCaret.current, nextCaret.current);
      setCaret(nextCaret.current);
      nextCaret.current = null;
    }
  }, [text]);

  function pick(option: MentionOption) {
    if (!mention) return;
    const next = insertMention(text, caret, mention, option.token);
    nextCaret.current = next.caret;
    setText(next.text);
    boxRef.current?.focus();
  }

  function submit() {
    if (!canSend) return;
    store.send(channelId, text, threadRootId ? { thread_root_id: threadRootId } : {});
    setText("");
    setCaret(0);
    boxRef.current?.focus();
  }

  function onChange(e: React.ChangeEvent<HTMLTextAreaElement>) {
    setText(e.target.value);
    setCaret(e.target.selectionStart);
    setActive(0);
    setDismissed(false);
    // Tell the others I am typing (not in a thread: its indicator is the channel's).
    const now = Date.now();
    if (!threadRootId && e.target.value.trim() !== "" && now - lastTyping.current >= typingEveryMs) {
      lastTyping.current = now;
      chat?.realtime.send("typing", { channel_id: channelId });
    }
  }

  function onKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (open) {
      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        e.preventDefault();
        const step = e.key === "ArrowDown" ? 1 : -1;
        setActive((current + step + shown.length) % shown.length);
        return;
      }
      if (e.key === "Enter" || e.key === "Tab") {
        e.preventDefault();
        pick(shown[current]!);
        return;
      }
      if (e.key === "Escape") {
        e.preventDefault();
        setDismissed(true);
        return;
      }
    }
    // Enter while an IME (Thai, Japanese) is composing confirms the word, it does not send.
    if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault();
      submit();
    }
  }

  const listId = `${uid}-mentions`;
  return (
    <form
      className="chat-composer"
      onSubmit={(e) => {
        e.preventDefault();
        submit();
      }}
    >
      <label className="visually-hidden" htmlFor={`${uid}-box`}>
        {label ?? t("chat.composer.label")}
      </label>
      <div className="chat-composer-box">
        {open && (
          <MentionList id={listId} label={t("chat.mention.list")} options={shown} active={current} onPick={pick} />
        )}
        <textarea
          id={`${uid}-box`}
          ref={boxRef}
          rows={2}
          value={text}
          onChange={onChange}
          onKeyDown={onKeyDown}
          onSelect={(e) => setCaret(e.currentTarget.selectionStart)}
          aria-describedby={`${uid}-hint ${uid}-count`}
          aria-controls={open ? listId : undefined}
          aria-activedescendant={open ? `${listId}-${current}` : undefined}
          aria-invalid={tooLong || undefined}
        />
      </div>
      <div className="chat-composer-bar">
        <small id={`${uid}-hint`} className="muted">
          {t("chat.composer.hint")}
        </small>
        <small id={`${uid}-count`} className={tooLong ? "form-error" : "muted"}>
          {t("chat.counter", { n: length.toLocaleString("en-US") })}
        </small>
        <button type="submit" disabled={!canSend}>
          {t("chat.send")}
        </button>
      </div>
      <p className="form-error" role="alert">
        {tooLong ? t("chat.tooLong") : null}
      </p>
    </form>
  );
}
