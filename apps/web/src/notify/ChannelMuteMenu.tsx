import { Bell, BellOff, EllipsisVertical } from "lucide-react";
import type { ReactNode } from "react";
import { Button } from "@/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import type { MenuItem } from "../chat/pins";
import { useT } from "../i18n/useT";
import { useModuleEnabled } from "../modules";
import { useChannelMute } from "./usePreferences";

type ChannelMute = ReturnType<typeof useChannelMute>;

/**
 * The Mute or Unmute entry of a conversation's menu (NOTI-07). Absent while notify is off. `mute` comes
 * from `useChannelMute` in a component that outlives the menu: a menu unmounts as soon as an entry is
 * chosen, and the toast of a request that is still running would go with it.
 */
export function MuteMenuItem({ mute, Item = DropdownMenuItem }: { mute: ChannelMute; Item?: MenuItem }) {
  const t = useT();
  const enabled = useModuleEnabled("notify");
  if (!enabled) return null;
  return (
    <Item disabled={!mute.ready || mute.pending} onSelect={mute.toggle}>
      {mute.muted ? <Bell aria-hidden="true" /> : <BellOff aria-hidden="true" />}
      {t(mute.muted ? "notify.unmute" : "notify.mute")}
    </Item>
  );
}

/**
 * The channel header's options menu: Mute and Unmute (NOTI-07), after any `children` entries the
 * page adds (Pin). Without children it is absent while notify is off.
 */
export function ChannelMuteMenu({ channelId, children }: { channelId: string; children?: ReactNode }) {
  const t = useT();
  const enabled = useModuleEnabled("notify");
  const mute = useChannelMute(channelId);
  if (!enabled && !children) return null;
  return (
    <DropdownMenu>
      <Tooltip>
        <TooltipTrigger asChild>
          <DropdownMenuTrigger asChild>
            <Button type="button" variant="ghost" size="icon-sm" aria-label={t("notify.channelOptions")}>
              <EllipsisVertical aria-hidden="true" />
            </Button>
          </DropdownMenuTrigger>
        </TooltipTrigger>
        <TooltipContent>{t("notify.channelOptions")}</TooltipContent>
      </Tooltip>
      <DropdownMenuContent align="end">
        {children}
        <MuteMenuItem mute={mute} />
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
