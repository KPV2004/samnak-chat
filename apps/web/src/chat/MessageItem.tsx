import type { ChatMessage, Reaction } from "@samnak/core";
import { ListTodo, MessageSquare, MessageSquareReply, Paperclip, Pencil, SmilePlus, Trash2 } from "lucide-react";
import { memo, useState, type ReactNode } from "react";
import { useSearchParams } from "react-router";
import { Button } from "@/components/ui/button";
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { toast } from "sonner";
import { UserAvatar } from "../components/UserAvatar";
import { useCurrentLocale, useT } from "../i18n/useT";
import { formatDateTime, formatMessageTime } from "../lib/format";
import { useChat } from "../state/chat";
import { canManage, useWorkspaces } from "../state/queries";
import { useChatExtensions } from "../task/api";
import { CreateTaskForm } from "../task/CreateTaskForm";
import { CardBody } from "./cards";
import { EditForm } from "./EditForm";
import { bodyClass, Markdown } from "./markdown";
import { useNameOf } from "./names";

const quickReactions = ["👍", "❤️", "😂", "🎉", "🙏", "👀"];

interface Props {
  message: ChatMessage;
  channelId: string;
  /** Inside the thread panel: no reply button, and the replies are not counted again. */
  inThread?: boolean;
  /** Same person as the message above, shortly after it: no header, the time shows on hover. */
  continued?: boolean;
  /** A day divider above this message, already labelled. */
  divider?: string;
}

export const MessageItem = memo(function MessageItem({ message: m, channelId, inThread = false, continued = false, divider }: Props) {
  const t = useT();
  const locale = useCurrentLocale();
  const chat = useChat();
  const nameOf = useNameOf();
  const role = useWorkspaces().current?.role;
  const [, setParams] = useSearchParams();
  const [editing, setEditing] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [deleteFailed, setDeleteFailed] = useState(false);
  const [picking, setPicking] = useState(false);
  const [creatingTask, setCreatingTask] = useState(false);
  const [failed, setFailed] = useState(false);
  const { messageActions } = useChatExtensions();
  const canCreateTask = messageActions.some((a) => a.id === "task.create_from_message");

  const me = chat?.userId ?? "";
  const mine = m.sender_id === me;
  const deleted = m.deleted_at !== null;
  const sent = m.status === "sent";
  const canDelete = sent && !deleted && (mine || canManage(role));
  const name = nameOf(m.sender_id);

  function openThread() {
    setParams((prev) => {
      const next = new URLSearchParams(prev);
      next.set("thread", m.id);
      return next;
    });
  }

  async function remove() {
    if (!chat) return;
    setDeleteFailed(false);
    try {
      await chat.api.deleteMessage(m.id);
      chat.store.applyMessage({ ...m, body: "", deleted_at: new Date().toISOString() });
      setConfirming(false);
      toast.success(t("chat.toast.deleted"));
    } catch {
      setDeleteFailed(true);
    }
  }

  /** Changes my reaction at once, and puts the message back if the server refuses. */
  async function react(emoji: string) {
    if (!chat) return;
    const had = m.reactions.find((r) => r.emoji === emoji);
    const adding = !had?.me;
    chat.store.applyMessage({ ...m, reactions: nextReactions(m.reactions, emoji, adding) });
    setPicking(false);
    setFailed(false);
    try {
      if (adding) await chat.api.addReaction(m.id, emoji);
      else await chat.api.removeReaction(m.id, emoji);
    } catch {
      chat.store.applyMessage(m);
      setFailed(true);
    }
  }

  return (
    <li tabIndex={-1} className="group relative outline-none focus-within:[&>.row]:bg-muted/50">
      {divider && (
        <div role="separator" aria-label={divider} className="my-3 flex items-center gap-3 text-xs font-medium text-muted-foreground">
          <span className="h-px flex-1 bg-border" aria-hidden="true" />
          <span>{divider}</span>
          <span className="h-px flex-1 bg-border" aria-hidden="true" />
        </div>
      )}
      <div
        className={cn(
          "row flex gap-3 rounded-md px-2 py-0.5 hover:bg-muted/50 md:px-3",
          !continued && !divider && "mt-2",
          !continued && divider && "mt-0",
          m.status === "pending" && "opacity-60",
        )}
      >
        <div className="w-12 shrink-0 pt-0.5">
          {continued ? (
            sent && (
              <time
                dateTime={m.created_at}
                title={formatDateTime(m.created_at, locale)}
                className="invisible block text-right text-[11px] whitespace-nowrap text-muted-foreground group-focus-within:visible group-hover:visible"
              >
                {formatMessageTime(m.created_at, locale)}
              </time>
            )
          ) : (
            <UserAvatar name={name} className="size-8" />
          )}
        </div>
        <div className="min-w-0 flex-1">
          {continued ? (
            <span className="sr-only">{name}</span>
          ) : (
            <div className="flex flex-wrap items-baseline gap-x-2">
              <strong className="text-sm font-semibold">{name}</strong>
              {sent && (
                <time dateTime={m.created_at} title={formatDateTime(m.created_at, locale)} className="text-xs text-muted-foreground">
                  {formatMessageTime(m.created_at, locale)}
                </time>
              )}
            </div>
          )}

          {deleted ? (
            <p className="m-0 text-sm italic text-muted-foreground">{t("chat.deleted")}</p>
          ) : editing ? (
            <EditForm message={m} onDone={() => setEditing(false)} />
          ) : m.kind !== "text" ? (
            <div className="mt-1">
              <CardBody message={m} />
            </div>
          ) : (
            m.body !== "" && (
              <div className={cn("mt-0.5 text-sm", bodyClass)}>
                <Markdown text={m.body} nameOf={nameOf} me={me} />
              </div>
            )
          )}
          {!deleted && m.edited_at && !editing && <span className="text-xs text-muted-foreground"> {t("chat.edited")}</span>}

          {!deleted && m.attachments.length > 0 && (
            <ul className="mt-2 flex flex-wrap gap-2" aria-label={t("chat.attachments")}>
              {m.attachments.map((a) => (
                <li key={a.id} className="inline-flex items-center gap-1.5 rounded-md border bg-background px-2 py-1 text-xs">
                  <Paperclip className="size-3.5 text-muted-foreground" aria-hidden="true" />
                  {a.file_name}
                </li>
              ))}
            </ul>
          )}

          {!deleted && m.reactions.length > 0 && (
            <ul className="mt-1.5 flex flex-wrap gap-1">
              {m.reactions.map((r) => (
                <li key={r.emoji}>
                  <button
                    type="button"
                    aria-pressed={r.me}
                    aria-label={t("chat.reactionLabel", { emoji: r.emoji, n: r.count })}
                    onClick={() => void react(r.emoji)}
                    className={cn(
                      "inline-flex h-6 items-center gap-1 rounded-full border px-2 text-xs transition-colors hover:bg-accent focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none",
                      r.me ? "border-primary/40 bg-primary/10 text-primary" : "bg-background",
                    )}
                  >
                    {t("chat.reactionToggle", { emoji: r.emoji, n: r.count })}
                  </button>
                </li>
              ))}
            </ul>
          )}

          {!inThread && m.reply_count > 0 && !deleted && (
            <button
              type="button"
              onClick={openThread}
              className="mt-1 inline-flex items-center gap-1 rounded-md px-1 text-xs font-medium text-primary hover:underline focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none"
            >
              <MessageSquare className="size-3.5" aria-hidden="true" />
              {m.reply_count === 1 ? t("chat.replyCount.one") : t("chat.replyCount.other", { n: m.reply_count })}
            </button>
          )}

          {m.status === "pending" && <small className="text-xs text-muted-foreground">{t("chat.sending")}</small>}
          {m.status === "failed" && mine && <FailedActions channelId={channelId} clientMsgId={m.id} />}

          {picking && (
            <ul aria-label={t("chat.reactionPicker")} className="mt-1.5 flex w-fit gap-0.5 rounded-full border bg-popover p-0.5 shadow-sm">
              {quickReactions.map((emoji) => (
                <li key={emoji}>
                  <button
                    type="button"
                    onClick={() => void react(emoji)}
                    className="flex size-7 items-center justify-center rounded-full text-base hover:bg-accent focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none"
                  >
                    {emoji}
                  </button>
                </li>
              ))}
            </ul>
          )}

          {creatingTask && (
            <div className="mt-2 rounded-lg border bg-card p-3">
              <CreateTaskForm message={m} channelId={channelId} onDone={() => setCreatingTask(false)} />
            </div>
          )}

          {failed && (
            <p className="mt-1 text-xs text-destructive" role="alert">
              {t("chat.actionError")}
            </p>
          )}
        </div>
      </div>

      {sent && !deleted && !editing && (
        <div
          role="group"
          aria-label={t("chat.messageActions")}
          className="absolute -top-3 right-3 flex items-center gap-0.5 rounded-md border bg-popover p-0.5 shadow-sm opacity-0 transition-opacity group-focus-within:opacity-100 group-hover:opacity-100"
        >
          <IconAction label={t("chat.react")} expanded={picking} onClick={() => setPicking((open) => !open)}>
            <SmilePlus />
          </IconAction>
          {!inThread && m.thread_root_id === null && (
            <IconAction label={t("chat.reply")} onClick={openThread}>
              <MessageSquareReply />
            </IconAction>
          )}
          {canCreateTask && m.kind === "text" && m.body !== "" && (
            <IconAction label={t("task.createFromMessage")} expanded={creatingTask} onClick={() => setCreatingTask((open) => !open)}>
              <ListTodo />
            </IconAction>
          )}
          {mine && (
            <IconAction label={t("chat.edit")} onClick={() => setEditing(true)}>
              <Pencil />
            </IconAction>
          )}
          {canDelete && (
            <IconAction label={t("chat.delete")} destructive onClick={() => setConfirming(true)}>
              <Trash2 />
            </IconAction>
          )}
        </div>
      )}

      <Dialog
        open={confirming}
        onOpenChange={(open) => {
          setConfirming(open);
          if (!open) setDeleteFailed(false);
        }}
      >
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>{t("chat.deleteConfirm")}</DialogTitle>
            <DialogDescription>{t("chat.deleteHint")}</DialogDescription>
          </DialogHeader>
          {deleteFailed && (
            <p className="text-sm text-destructive" role="alert">
              {t("chat.actionError")}
            </p>
          )}
          <DialogFooter>
            <DialogClose asChild>
              <Button type="button" variant="outline">
                {t("cancel")}
              </Button>
            </DialogClose>
            <Button type="button" variant="destructive" onClick={() => void remove()}>
              {t("confirm")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </li>
  );
});

function nextReactions(reactions: Reaction[], emoji: string, adding: boolean): Reaction[] {
  const found = reactions.some((r) => r.emoji === emoji);
  if (adding && !found) return [...reactions, { emoji, count: 1, me: true }];
  return reactions
    .map((r) => (r.emoji === emoji ? { ...r, count: r.count + (adding ? 1 : -1), me: adding } : r))
    .filter((r) => r.count > 0);
}

/** An icon button of the hover toolbar: the label is its name and its tooltip. */
function IconAction({
  label,
  expanded,
  destructive = false,
  onClick,
  children,
}: {
  label: string;
  expanded?: boolean;
  destructive?: boolean;
  onClick(): void;
  children: ReactNode;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          aria-label={label}
          aria-expanded={expanded}
          onClick={onClick}
          className={cn(destructive && "hover:text-destructive")}
        >
          {children}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

function FailedActions({ channelId, clientMsgId }: { channelId: string; clientMsgId: string }) {
  const t = useT();
  const store = useChat()?.store;
  return (
    <div className="mt-1 flex flex-wrap items-center gap-2 text-xs" role="group">
      <span className="text-destructive">{t("chat.failed")}</span>
      <Button type="button" variant="outline" size="xs" onClick={() => void store?.retry(channelId, clientMsgId)}>
        {t("retry")}
      </Button>
      <Button type="button" variant="ghost" size="xs" onClick={() => store?.discard(channelId, clientMsgId)}>
        {t("chat.discard")}
      </Button>
    </div>
  );
}
