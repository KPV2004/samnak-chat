import type { ChatCommand } from "@samnak/core";
import { useQuery } from "@tanstack/react-query";
import { SendHorizontal } from "lucide-react";
import { useId, useLayoutEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";
import { useT } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { useChat } from "../state/chat";
import { useChatExtensions, useExtensionsApi } from "../task/api";
import { MentionList } from "./MentionList";
import { addPick, channelText, insertMention, mentionAt, toTokens, type MentionOption, type Picks } from "./mentions";
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

/** What the box holds: the text as the sender sees it, and the mentions picked into it. */
interface Draft {
  text: string;
  picks: Picks;
}

const emptyDraft: Draft = { text: "", picks: new Map() };

/** What a slash command said back (shown to the sender only), or why it failed. */
interface Note {
  text: string;
  failed: boolean;
}

export function Composer({ channelId, threadRootId, label }: Props) {
  const t = useT();
  const chat = useChat();
  const store = chat?.store;
  const nameOf = useNameOf();
  const { commands } = useChatExtensions();
  const extensions = useExtensionsApi();
  const [draft, setDraft] = useState<Draft>(emptyDraft);
  const { text, picks } = draft;
  const [caret, setCaret] = useState(0);
  const [active, setActive] = useState(0);
  const [dismissed, setDismissed] = useState(false);
  const [running, setRunning] = useState(false);
  const [note, setNote] = useState<Note | null>(null);
  const boxRef = useRef<HTMLTextAreaElement>(null);
  const nextCaret = useRef<number | null>(null);
  // The caret right after the space a pick left behind, until the sender types something.
  const pickedEnd = useRef<number | null>(null);
  const lastTyping = useRef(0);
  const uid = useId();

  // What is sent: each picked name as its token. The counter and the limit count that.
  const body = toTokens(text, picks);
  const length = [...body].length;
  const tooLong = length > maxLength;
  const canSend = text.trim() !== "" && !tooLong && store !== undefined && !running;

  const members = useQuery({
    queryKey: ["chat-channel-members", channelId],
    enabled: chat !== null,
    staleTime: 30_000,
    queryFn: async () => (await chat!.api.listChannelMembers(channelId)).items,
  });

  // A command being typed: a slash and letters, with no space yet.
  const slash = !dismissed && /^\/\S*$/.test(text) ? text.slice(1).toLowerCase() : null;
  const mention = dismissed ? null : mentionAt(text, caret);
  const options: MentionOption[] = [];
  if (slash !== null) {
    for (const c of commands) {
      if (c.name.startsWith(slash)) options.push({ token: `/${c.name} `, label: c.usage, detail: c.description });
    }
  }
  if (mention) {
    const q = mention.query.toLowerCase();
    if ("channel".startsWith(q)) {
      options.push({ token: "<!channel>", label: t("chat.mentionChannel"), detail: t("chat.mention.channel"), insert: channelText });
    }
    for (const m of members.data ?? []) {
      const name = m.display_name || nameOf(m.user_id);
      if (m.user_id !== chat?.userId && name.toLowerCase().includes(q)) {
        options.push({ token: `<@${m.user_id}>`, label: name, insert: `@${name}` });
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
    if (slash !== null) {
      nextCaret.current = option.token.length;
      setDraft({ text: option.token, picks: new Map() });
      boxRef.current?.focus();
      return;
    }
    if (!mention) return;
    const added = addPick(picks, option.insert ?? option.token, option.token);
    const next = insertMention(text, caret, mention, added.shown);
    nextCaret.current = next.caret;
    pickedEnd.current = next.caret;
    setDraft({ text: next.text, picks: added.picks });
    boxRef.current?.focus();
  }

  async function submit() {
    if (!canSend) return;
    const command = commandIn(body, commands);
    if (!command) {
      store.send(channelId, body, threadRootId ? { thread_root_id: threadRootId } : {});
      setDraft(emptyDraft);
      setCaret(0);
      boxRef.current?.focus();
      return;
    }
    const sent = text;
    setRunning(true);
    setNote(null);
    try {
      const result = await extensions.runCommand(channelId, {
        name: command.name,
        args: command.args,
        thread_root_id: threadRootId,
      });
      // Clears the box unless the sender has typed something else meanwhile.
      setDraft((current) => (current.text === sent ? emptyDraft : current));
      if (result.text) setNote({ text: result.text, failed: false });
    } catch (err) {
      setNote({ text: t(errorKey(err)), failed: true });
    } finally {
      setRunning(false);
    }
  }

  function onChange(e: React.ChangeEvent<HTMLTextAreaElement>) {
    const value = e.target.value;
    pickedEnd.current = null;
    setDraft((current) => ({ ...current, text: value }));
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
      // A command already typed in full sends on Enter instead of being picked again.
      const complete = slash !== null && shown[current]!.token.trim() === text.trim();
      if (e.key === "Tab" || (e.key === "Enter" && !complete)) {
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
    // The pick already left a space after the name; a space typed right after it is dropped, not doubled.
    if (e.key === " " && pickedEnd.current === caret && text.charAt(caret - 1) === " ") {
      e.preventDefault();
      return;
    }
    // Enter while an IME (Thai, Japanese) is composing confirms the word, it does not send.
    if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault();
      void submit();
    }
  }

  const listId = `${uid}-mentions`;
  return (
    <form
      className="grid gap-1 border-t bg-background p-2 md:p-3"
      onSubmit={(e) => {
        e.preventDefault();
        void submit();
      }}
    >
      <label className="sr-only" htmlFor={`${uid}-box`}>
        {label ?? t("chat.composer.label")}
      </label>
      <div
        className={cn(
          "relative rounded-lg border border-input bg-background shadow-xs transition-[color,box-shadow] focus-within:border-ring focus-within:ring-[3px] focus-within:ring-ring/50",
          tooLong && "border-destructive ring-destructive/20",
        )}
      >
        {open && (
          <MentionList
            id={listId}
            label={slash !== null ? t("chat.command.list") : t("chat.mention.list")}
            options={shown}
            active={current}
            onPick={pick}
          />
        )}
        <Textarea
          id={`${uid}-box`}
          ref={boxRef}
          rows={1}
          value={text}
          onChange={onChange}
          onKeyDown={onKeyDown}
          onSelect={(e) => setCaret(e.currentTarget.selectionStart)}
          aria-describedby={`${uid}-hint ${uid}-count`}
          aria-controls={open ? listId : undefined}
          aria-activedescendant={open ? `${listId}-${current}` : undefined}
          aria-invalid={tooLong || undefined}
          className="max-h-40 min-h-11 resize-none border-0 bg-transparent px-3 py-2.5 shadow-none focus-visible:border-0 focus-visible:ring-0 dark:bg-transparent"
        />
        <div className="flex items-center gap-2 px-2 pb-2">
          <small id={`${uid}-hint`} className="min-w-0 flex-1 text-xs text-muted-foreground">
            {t("chat.composer.hint")}
          </small>
          <small
            id={`${uid}-count`}
            className={cn("shrink-0 text-xs tabular-nums", tooLong ? "text-destructive" : "text-muted-foreground")}
          >
            {t("chat.counter", { n: length.toLocaleString("en-US") })}
          </small>
          <Button type="submit" size="sm" disabled={!canSend}>
            <SendHorizontal />
            {t("chat.send")}
          </Button>
        </div>
      </div>
      <p className="text-xs text-destructive empty:hidden" role="alert">
        {tooLong ? t("chat.tooLong") : null}
      </p>
      {note && (
        <p className={cn("text-xs", note.failed ? "text-destructive" : "text-muted-foreground")} role={note.failed ? "alert" : "status"}>
          {note.text}
        </p>
      )}
    </form>
  );
}

/** The command a message starts with, when the server registered it: `/name args`. */
function commandIn(text: string, commands: ChatCommand[]): { name: string; args: string } | null {
  const match = /^\/(\S+)\s*([\s\S]*)$/.exec(text.trim());
  if (!match || !commands.some((c) => c.name === match[1])) return null;
  return { name: match[1]!, args: match[2]!.trim() };
}
