import { Bell, BellOff, EllipsisVertical } from "lucide-react";
import { Button } from "@/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { useT } from "../i18n/useT";
import { useModuleEnabled } from "../modules";
import { useChannelMute } from "./usePreferences";

/** The channel header's options menu, which holds Mute and Unmute (NOTI-07). Absent while notify is off. */
export function ChannelMuteMenu({ channelId }: { channelId: string }) {
  const t = useT();
  const enabled = useModuleEnabled("notify");
  const mute = useChannelMute(channelId);
  if (!enabled) return null;
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
        <DropdownMenuItem disabled={!mute.ready || mute.pending} onSelect={mute.toggle}>
          {mute.muted ? <Bell aria-hidden="true" /> : <BellOff aria-hidden="true" />}
          {t(mute.muted ? "notify.unmute" : "notify.mute")}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
