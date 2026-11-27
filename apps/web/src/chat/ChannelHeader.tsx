import type { Channel } from "@samnak/core";
import { useQuery } from "@tanstack/react-query";
import { ArrowLeft, Link2, Users } from "lucide-react";
import { Link } from "react-router";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { DropdownMenuItem } from "@/components/ui/dropdown-menu";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { useT } from "../i18n/useT";
import { ChannelMuteMenu } from "../notify/ChannelMuteMenu";
import { useChat } from "../state/chat";
import { ConversationAvatar, PersonAvatar } from "./ConversationAvatar";
import { IconTool } from "./IconTool";
import { channelTitle, useAvatarOf, useNameOf } from "./names";
import { PinMenuItem } from "./pins";

/** The top of a channel: what it is called, its topic, who is in it, a link to it and its options. */
export function ChannelHeader({ channel }: { channel: Channel }) {
  const t = useT();
  const nameOf = useNameOf();
  const me = useChat()?.userId ?? "";
  const title = channelTitle(channel, me, nameOf, t);

  async function copyLink() {
    try {
      await navigator.clipboard.writeText(`${window.location.origin}/chat/${channel.id}`);
      toast.success(t("chat.toast.linkCopied"));
    } catch {
      toast.error(t("chat.actionError"));
    }
  }

  return (
    <header className="flex min-h-14 shrink-0 items-center gap-3 border-b px-4 py-2">
      {/* On a phone the channel fills the screen, so its own header carries the way back to the list. */}
      <Link
        to="/chat"
        aria-label={t("chat.back")}
        className="grid size-9 shrink-0 place-items-center rounded-md text-muted-foreground outline-none transition-colors duration-150 hover:bg-accent hover:text-accent-foreground focus-visible:ring-2 focus-visible:ring-ring md:hidden"
      >
        <ArrowLeft aria-hidden="true" className="size-5" />
      </Link>
      <ConversationAvatar channel={channel} title={title} tone="page" className="size-9" />
      <div className="grid min-w-0 flex-1">
        <h1 className="text-base leading-snug font-semibold">{title}</h1>
        {channel.topic && (
          <p className="line-clamp-2 text-xs text-muted-foreground" title={channel.topic}>
            {channel.topic}
          </p>
        )}
      </div>
      <Members channel={channel} />
      <IconTool label={t("chat.copyLink")} size="icon-sm" onClick={() => void copyLink()}>
        <Link2 />
      </IconTool>
      <ChannelMuteMenu channelId={channel.id}>
        <PinMenuItem channel={channel} Item={DropdownMenuItem} />
      </ChannelMuteMenu>
    </header>
  );
}

/** The member count, which opens the list of who is in the channel. */
function Members({ channel }: { channel: Channel }) {
  const t = useT();
  const chat = useChat();
  const nameOf = useNameOf();
  const avatarOf = useAvatarOf();
  const members = useQuery({
    queryKey: ["chat-channel-members", channel.id],
    enabled: chat !== null,
    staleTime: 30_000,
    queryFn: async () => (await chat!.api.listChannelMembers(channel.id)).items,
  });
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button type="button" variant="outline" size="sm" className="shrink-0 text-xs">
          <Users aria-hidden="true" />
          {t("chat.members", { n: channel.member_count })}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-64 p-2" aria-label={t("chat.membersTitle")}>
        <p className="px-2 py-1 text-xs font-semibold text-muted-foreground">{t("chat.membersTitle")}</p>
        {(members.data ?? []).length === 0 ? (
          <p className="px-2 py-1 text-sm text-muted-foreground">{members.isPending ? t("loading") : t("chat.members.empty")}</p>
        ) : (
          <ul className="max-h-72 overflow-y-auto">
            {members.data!.map((m) => {
              const name = m.display_name || nameOf(m.user_id);
              return (
                <li key={m.user_id} className="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm">
                  <PersonAvatar id={m.user_id} name={name} src={avatarOf(m.user_id)} className="size-6 *:data-[slot=avatar-fallback]:text-xs" />
                  {name}
                </li>
              );
            })}
          </ul>
        )}
      </PopoverContent>
    </Popover>
  );
}
