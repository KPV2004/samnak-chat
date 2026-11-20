import type { ChatMessage, Reaction } from "@samnak/core";
import { Ellipsis, ListTodo, MessageSquare, MessageSquareReply, Paperclip, Pencil, SmilePlus, Ticket, Trash2 } from "lucide-react";
import { memo, useRef, useState } from "react";
import { useSearchParams } from "react-router";
import { Button } from "@/components/ui/button";
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { toast } from "sonner";
import { UserAvatar } from "../components/UserAvatar";
import { useCurrentLocale, useT } from "../i18n/useT";
import { formatDateTime, formatMessageTime } from "../lib/format";
import { useChat } from "../state/chat";
import { canManage, useWorkspaces } from "../state/queries";
import { CreateTicketFromMessage } from "../helpdesk/CreateTicketFromMessage";
import { useChatExtensions } from "../task/api";
import { CreateTaskForm } from "../task/CreateTaskForm";
import { CardBody } from "./cards";
import { clockTime } from "./grouping";
import { EditForm } from "./EditForm";
import { IconTool } from "./IconTool";
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
  /** The "new messages" line goes above this message: it is the first one the reader had not seen. */
  newLine?: boolean;
}

export const MessageItem = memo(function MessageItem({ message: m, channelId, inThread = false, continued = false, divider, newLine = false }: Props) {
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
  const [menuOpen, setMenuOpen] = useState(false);
  // Set when a menu item moves the focus elsewhere (a form, a dialog), so the menu does not take it back.
  const leaving = useRef(false);
  const [creatingTask, setCreatingTask] = useState(false);
  const [creatingTicket, setCreatingTicket] = useState(false);
  const [failed, setFailed] = useState(false);
  const { messageActions } = useChatExtensions();
  const canCreateTask = messageActions.some((a) => a.id === "task.create_from_message");
  const canCreateTicket = messageActions.some((a) => a.id === "helpdesk.create_from_message");

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

  const pinned = picking || menuOpen;
  return (
    <li tabIndex={-1} className="group relative outline-none">
      {divider && (
        <div role="separator" aria-label={divider} className="my-3 flex items-center px-2 md:px-3">
          <span className="h-px flex-1 bg-border" aria-hidden="true" />
          <span className="mx-3 rounded-full border bg-background px-3 py-0.5 text-xs font-medium text-muted-foreground">{divider}</span>
          <span className="h-px flex-1 bg-border" aria-hidden="true" />
        </div>
      )}
      {newLine && (
        <div role="separator" aria-label={t("chat.newMessages")} className="my-2 flex items-center gap-2 px-2 md:px-3">
          <span className="h-px flex-1 bg-primary/40" aria-hidden="true" />
          <span className="rounded-full bg-primary/10 px-2 py-0.5 text-xs font-semibold text-primary">{t("chat.newMessages")}</span>
        </div>
      )}
      <div
        className={cn(
          "row grid grid-cols-[2.25rem_minmax(0,1fr)] gap-x-3 rounded-md px-2 py-0.5 transition-colors hover:bg-muted/60 group-focus-within:bg-muted/60 md:px-3",
          pinned && "bg-muted/60",
          !continued && !divider && !newLine && "mt-2",
          m.status === "pending" && "opacity-60",
        )}
      >
        <div className="pt-0.5">
          {continued ? (
            sent && (
              <time
                dateTime={m.created_at}
                title={formatDateTime(m.created_at, locale)}
                className="invisible block text-right text-[11px] leading-6 whitespace-nowrap text-muted-foreground group-focus-within:visible group-hover:visible"
              >
                {clockTime(m.created_at, locale)}
              </time>
            )
          ) : (
            <UserAvatar name={name} className="size-9" />
          )}
        </div>
        <div className="min-w-0">
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
            <div className="mt-1 max-w-xl">
              <CardBody message={m} />
            </div>
          ) : (
            m.body !== "" && (
              <div className={cn("text-sm", !continued && "mt-0.5", bodyClass)}>
                <Markdown text={m.body} nameOf={nameOf} me={me} />
              </div>
            )
          )}
          {!deleted && m.edited_at && !editing && <span className="text-xs text-muted-foreground"> {t("chat.edited")}</span>}

          {!deleted && m.attachments.length > 0 && (
            <ul className="mt-2 flex flex-wrap gap-2" aria-label={t("chat.attachments")}>
              {m.attachments.map((a) => (
                <li key={a.id} className="inline-flex items-center gap-1.5 rounded-md border bg-card px-2.5 py-1.5 text-xs">
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
                      "inline-flex h-6 items-center gap-1 rounded-full border px-2 text-xs transition-[color,background-color,scale] duration-150 outline-none motion-safe:active:scale-[0.97] focus-visible:ring-[3px] focus-visible:ring-ring/50",
                      r.me
                        ? "border-primary/40 bg-primary/10 text-primary hover:bg-primary/15 active:bg-primary/20"
                        : "bg-card hover:bg-accent active:bg-accent/70",
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
              className="mt-1 -ml-1 inline-flex items-center gap-1.5 rounded-md px-1.5 py-0.5 text-xs font-medium text-primary transition-colors outline-none hover:bg-accent active:bg-accent/70 focus-visible:ring-[3px] focus-visible:ring-ring/50"
            >
              <MessageSquare className="size-3.5" aria-hidden="true" />
              {m.reply_count === 1 ? t("chat.replyCount.one") : t("chat.replyCount.other", { n: m.reply_count })}
            </button>
          )}

          {m.status === "pending" && <small className="text-xs text-muted-foreground">{t("chat.sending")}</small>}
          {m.status === "failed" && mine && <FailedActions channelId={channelId} clientMsgId={m.id} />}

          {creatingTask && (
            <div className="mt-2 rounded-lg border bg-card p-3">
              <CreateTaskForm message={m} channelId={channelId} onDone={() => setCreatingTask(false)} />
            </div>
          )}

          {creatingTicket && <CreateTicketFromMessage message={m} onDone={() => setCreatingTicket(false)} />}

          {failed && (
            <p className="mt-1 text-xs text-destructive" role="alert">
              {t("chat.actionError")}
            </p>
          )}
        </div>
      </div>

      {picking && (
        <ul aria-label={t("chat.reactionPicker")} className="absolute top-4 right-3 z-20 flex w-fit gap-0.5 rounded-full border bg-popover p-0.5 shadow-md">
          {quickReactions.map((emoji) => (
            <li key={emoji}>
              <button
                type="button"
                onClick={() => void react(emoji)}
                className="flex size-7 items-center justify-center rounded-full text-base transition-colors outline-none hover:bg-accent active:bg-accent/70 focus-visible:ring-[3px] focus-visible:ring-ring/50"
              >
                {emoji}
              </button>
            </li>
          ))}
        </ul>
      )}

      {sent && !deleted && !editing && (
        <div
          role="group"
          aria-label={t("chat.messageActions")}
          className={cn(
            "pointer-events-none absolute -top-3 right-3 z-10 flex items-center gap-0.5 rounded-md border bg-popover p-0.5 text-popover-foreground opacity-0 shadow-sm transition-opacity group-focus-within:pointer-events-auto group-focus-within:opacity-100 group-hover:pointer-events-auto group-hover:opacity-100",
            pinned && "pointer-events-auto opacity-100",
          )}
        >
          <IconTool label={t("chat.react")} expanded={picking} onClick={() => setPicking((open) => !open)}>
            <SmilePlus />
          </IconTool>
          {!inThread && m.thread_root_id === null && (
            <IconTool label={t("chat.reply")} onClick={openThread}>
              <MessageSquareReply />
            </IconTool>
          )}
          {canCreateTask && m.kind === "text" && m.body !== "" && (
            <IconTool label={t("task.createFromMessage")} expanded={creatingTask} onClick={() => setCreatingTask((open) => !open)}>
              <ListTodo />
            </IconTool>
          )}
          {canCreateTicket && m.kind === "text" && m.body !== "" && (
            <IconTool label={t("helpdesk.createFromMessage")} expanded={creatingTicket} onClick={() => setCreatingTicket((open) => !open)}>
              <Ticket />
            </IconTool>
          )}
          {(mine || canDelete) && (
            <DropdownMenu modal={false} open={menuOpen} onOpenChange={setMenuOpen}>
              <Tooltip>
                <TooltipTrigger asChild>
                  <DropdownMenuTrigger asChild>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon-xs"
                      aria-label={t("chat.more")}
                      className="text-muted-foreground hover:text-foreground"
                    >
                      <Ellipsis />
                    </Button>
                  </DropdownMenuTrigger>
                </TooltipTrigger>
                <TooltipContent>{t("chat.more")}</TooltipContent>
              </Tooltip>
              <DropdownMenuContent
                align="end"
                onCloseAutoFocus={(e) => {
                  if (leaving.current) e.preventDefault();
                  leaving.current = false;
                }}
              >
                {mine && (
                  <DropdownMenuItem
                    onSelect={() => {
                      leaving.current = true;
                      setEditing(true);
                    }}
                  >
                    <Pencil />
                    {t("chat.edit")}
                  </DropdownMenuItem>
                )}
                {canDelete && (
                  <DropdownMenuItem
                    variant="destructive"
                    onSelect={() => {
                      leaving.current = true;
                      setConfirming(true);
                    }}
                  >
                    <Trash2 />
                    {t("chat.delete")}
                  </DropdownMenuItem>
                )}
              </DropdownMenuContent>
            </DropdownMenu>
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
