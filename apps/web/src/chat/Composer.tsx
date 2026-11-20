import type { ChatCommand } from "@samnak/core";
import { useQuery } from "@tanstack/react-query";
import { AtSign, Bold, Code, Italic, Link2, Paperclip, SendHorizontal, Smile, X } from "lucide-react";
import { useId, useLayoutEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Textarea } from "@/components/ui/textarea";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { useT } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { useChat } from "../state/chat";
import { useChatExtensions, useExtensionsApi } from "../task/api";
import { MentionList } from "./MentionList";
import { addPick, channelText, insertMention, mentionAt, toTokens, type MentionOption, type Picks } from "./mentions";
import { insertAt, wrapSelection, type Edit, type Mark } from "./format";
import { IconTool } from "./IconTool";
import { useNameOf } from "./names";
import { maxFileBytes, useChannelUpload } from "./uploads";

export const maxLength = 4000;
/** The server fans typing out at most this often, so the client does not send more. */
const typingEveryMs = 3000;
const maxOptions = 6;

const emojis = [
  "😀", "😂", "🙂", "😉", "😍", "🤔", "😅", "😢", "😮", "😎", "🙌", "👏",
  "👍", "👎", "🙏", "💪", "👀", "🎉", "🔥", "✅", "❌", "⭐", "💡", "❤️",
];

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

/** A file chosen for the message: its attachment id is known once it is uploaded. */
interface Attached {
  key: number;
  name: string;
  id: string | null;
}

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
  const [files, setFiles] = useState<Attached[]>([]);
  const [emojiOpen, setEmojiOpen] = useState(false);
  const upload = useChannelUpload();
  const boxRef = useRef<HTMLTextAreaElement>(null);
  const nextFileKey = useRef(0);
  const nextSelection = useRef<[number, number] | null>(null);
  const lastTyping = useRef(0);
  const uid = useId();

  // What is sent: each picked name as its token. The counter and the limit count that.
  const body = toTokens(text, picks);
  const length = [...body].length;
  const tooLong = length > maxLength;
  const uploading = files.some((f) => f.id === null);
  const attachmentIds = files.flatMap((f) => (f.id === null ? [] : [f.id]));
  const canSend = (text.trim() !== "" || attachmentIds.length > 0) && !uploading && !tooLong && store !== undefined && !running;

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

  // Puts the caret (or the selection) where an insert or a format said once the new text is in the box.
  useLayoutEffect(() => {
    if (nextSelection.current !== null && boxRef.current) {
      const [start, end] = nextSelection.current;
      boxRef.current.setSelectionRange(start, end);
      setCaret(end);
      nextSelection.current = null;
    }
  }, [text]);

  /** Shows an edit of the text in the box and puts the caret where the edit says. */
  function applyEdit(edit: Edit) {
    nextSelection.current = [edit.start, edit.end];
    setDraft((current) => ({ ...current, text: edit.text }));
    boxRef.current?.focus();
  }

  function format(mark: Mark) {
    const el = boxRef.current;
    if (el) applyEdit(wrapSelection(text, el.selectionStart, el.selectionEnd, mark));
  }

  function insert(what: string) {
    const el = boxRef.current;
    if (el) applyEdit(insertAt(text, el.selectionStart, el.selectionEnd, what));
  }

  function mentionSomeone() {
    const before = text.slice(0, boxRef.current?.selectionStart ?? text.length);
    insert(before === "" || /\s$/.test(before) ? "@" : " @");
  }

  function attach(list: FileList | null) {
    if (!list) return;
    setNote(null);
    for (const file of Array.from(list)) {
      if (file.size > maxFileBytes) {
        setNote({ text: t("chat.attach.tooBig"), failed: true });
        continue;
      }
      const key = nextFileKey.current++;
      setFiles((current) => [...current, { key, name: file.name, id: null }]);
      upload(channelId, file).then(
        (id) => setFiles((current) => current.map((f) => (f.key === key ? { ...f, id } : f))),
        () => {
          setFiles((current) => current.filter((f) => f.key !== key));
          setNote({ text: t("chat.attach.error"), failed: true });
        },
      );
    }
  }

  function pick(option: MentionOption) {
    if (slash !== null) {
      nextSelection.current = [option.token.length, option.token.length];
      setDraft({ text: option.token, picks: new Map() });
      boxRef.current?.focus();
      return;
    }
    if (!mention) return;
    const added = addPick(picks, option.insert ?? option.token, option.token);
    const next = insertMention(text, caret, mention, added.shown);
    nextSelection.current = [next.caret, next.caret];
    setDraft({ text: next.text, picks: added.picks });
    boxRef.current?.focus();
  }

  async function submit() {
    if (!canSend) return;
    const command = attachmentIds.length > 0 ? null : commandIn(body, commands);
    if (!command) {
      store.send(channelId, body, {
        ...(threadRootId ? { thread_root_id: threadRootId } : {}),
        ...(attachmentIds.length > 0 ? { attachment_ids: attachmentIds } : {}),
      });
      setDraft(emptyDraft);
      setFiles([]);
      setNote(null);
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
    // Enter while an IME (Thai, Japanese) is composing confirms the word, it does not send.
    if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault();
      void submit();
    }
  }

  const listId = `${uid}-mentions`;
  return (
    <form
      className="shrink-0 px-3 pb-3 md:px-4 md:pb-4"
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
          "relative rounded-lg border border-input bg-card transition-[color,box-shadow] focus-within:border-ring focus-within:ring-[3px] focus-within:ring-ring/50",
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
        <div role="toolbar" aria-label={t("chat.format.label")} className="flex items-center gap-0.5 border-b px-1.5 py-1">
          <IconTool label={t("chat.format.bold")} onClick={() => format("bold")}>
            <Bold />
          </IconTool>
          <IconTool label={t("chat.format.italic")} onClick={() => format("italic")}>
            <Italic />
          </IconTool>
          <IconTool label={t("chat.format.code")} onClick={() => format("code")}>
            <Code />
          </IconTool>
          <IconTool label={t("chat.format.link")} onClick={() => format("link")}>
            <Link2 />
          </IconTool>
        </div>
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
          className="max-h-40 min-h-11 resize-none rounded-none border-0 bg-transparent px-3 py-2.5 shadow-none focus-visible:border-0 focus-visible:ring-0 dark:bg-transparent"
        />
        {files.length > 0 && (
          <ul aria-label={t("chat.attach.list")} className="flex flex-wrap gap-1.5 px-2 pb-2">
            {files.map((f) => (
              <li
                key={f.key}
                className={cn("inline-flex h-7 max-w-full items-center gap-1.5 rounded-md border bg-background pr-1 pl-2 text-xs", f.id === null && "text-muted-foreground")}
              >
                <Paperclip className="size-3.5 shrink-0 text-muted-foreground" aria-hidden="true" />
                <span className="min-w-0 break-all">{f.name}</span>
                {f.id === null && <span className="sr-only">{t("chat.attach.uploading")}</span>}
                <IconTool label={t("chat.attach.remove", { name: f.name })} onClick={() => setFiles((all) => all.filter((x) => x.key !== f.key))}>
                  <X />
                </IconTool>
              </li>
            ))}
          </ul>
        )}
        <div className="flex items-center gap-0.5 px-1.5 pb-1.5">
          <Tooltip>
            <TooltipTrigger asChild>
              <label className="inline-flex size-6 cursor-pointer items-center justify-center rounded-md text-muted-foreground transition-colors outline-none hover:bg-accent hover:text-foreground active:bg-accent/70 has-[:focus-visible]:ring-[3px] has-[:focus-visible]:ring-ring/50 [&_svg]:size-3">
                <Paperclip aria-hidden="true" />
                <input
                  type="file"
                  multiple
                  aria-label={t("chat.attach")}
                  className="sr-only"
                  onChange={(e) => {
                    attach(e.currentTarget.files);
                    e.currentTarget.value = "";
                  }}
                />
              </label>
            </TooltipTrigger>
            <TooltipContent>{t("chat.attach")}</TooltipContent>
          </Tooltip>
          <Popover open={emojiOpen} onOpenChange={setEmojiOpen}>
            <Tooltip>
              <TooltipTrigger asChild>
                <PopoverTrigger asChild>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-xs"
                    aria-label={t("chat.emoji")}
                    className="text-muted-foreground hover:text-foreground"
                  >
                    <Smile />
                  </Button>
                </PopoverTrigger>
              </TooltipTrigger>
              <TooltipContent>{t("chat.emoji")}</TooltipContent>
            </Tooltip>
            <PopoverContent
              align="start"
              side="top"
              className="w-auto p-1.5"
              aria-label={t("chat.emoji.list")}
              onCloseAutoFocus={(e) => {
                e.preventDefault();
                boxRef.current?.focus();
              }}
            >
              <ul className="grid grid-cols-6 gap-0.5">
                {emojis.map((emoji) => (
                  <li key={emoji}>
                    <button
                      type="button"
                      onClick={() => {
                        insert(emoji);
                        setEmojiOpen(false);
                      }}
                      className="flex size-8 items-center justify-center rounded-md text-lg transition-colors hover:bg-accent active:bg-accent/70 focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none"
                    >
                      {emoji}
                    </button>
                  </li>
                ))}
              </ul>
            </PopoverContent>
          </Popover>
          <IconTool label={t("chat.mention.add")} onClick={mentionSomeone}>
            <AtSign />
          </IconTool>
          <small id={`${uid}-count`} className={cn("ml-auto shrink-0 pr-2 text-xs tabular-nums", tooLong ? "text-destructive" : "text-muted-foreground")}>
            {t("chat.counter", { n: length.toLocaleString("en-US") })}
          </small>
          <Button type="submit" size="icon-sm" aria-label={t("chat.send")} disabled={!canSend}>
            <SendHorizontal />
          </Button>
        </div>
      </div>
      <small id={`${uid}-hint`} className="mt-1 hidden px-1 text-xs text-muted-foreground md:block">
        {t("chat.composer.hint")}
      </small>
      <p className="mt-1 text-xs text-destructive empty:hidden" role="alert">
        {tooLong ? t("chat.tooLong") : note?.failed ? note.text : null}
      </p>
      {note && !note.failed && (
        <p className="mt-1 text-xs text-muted-foreground" role="status">
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
