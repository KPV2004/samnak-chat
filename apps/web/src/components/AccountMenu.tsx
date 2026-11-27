import { BellRing, Languages, Laptop, LogOut, Monitor, Moon, Sun, UserRound } from "lucide-react";
import { Link } from "react-router";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { useT } from "@/i18n/useT";
import { cn } from "@/lib/utils";
import { useTheme, type ThemeChoice } from "@/state/theme";
import type { Locale } from "../i18n/messages";
import { useModuleEnabled } from "../modules";
import { useLocale } from "../state/locale";
import { useMe } from "../state/queries";
import { useSession } from "../state/session";
import { UserAvatar } from "./UserAvatar";

/**
 * The signed-in user at the foot of the rail: their avatar opens a menu with Profile, Devices,
 * Notification settings, theme, language and Sign out. `current` marks the avatar while an account
 * page is open.
 */
export function AccountMenu({ current, onNavigate }: { current: boolean; onNavigate?: () => void }) {
  const t = useT();
  const session = useSession();
  const me = useMe().data;
  const notify = useModuleEnabled("notify");
  const { theme, setTheme } = useTheme();
  const { locale, setLocale } = useLocale();
  if (!me) return null;
  return (
    <DropdownMenu>
      <Tooltip>
        <TooltipTrigger asChild>
          <DropdownMenuTrigger asChild>
            <button
              type="button"
              aria-label={t("account.menu")}
              className={cn(
                "grid size-9 place-items-center rounded-full outline-none transition-[box-shadow,scale] duration-150",
                "focus-visible:ring-2 focus-visible:ring-sidebar-ring focus-visible:ring-offset-2 focus-visible:ring-offset-sidebar",
                "hover:ring-2 hover:ring-sidebar-foreground/30 motion-safe:active:scale-95",
                current && "ring-2 ring-sidebar-primary",
              )}
            >
              <UserAvatar name={me.display_name} src={me.avatar_url} />
            </button>
          </DropdownMenuTrigger>
        </TooltipTrigger>
        <TooltipContent side="right">{t("account.menu")}</TooltipContent>
      </Tooltip>
      <DropdownMenuContent side="right" align="end" className="w-60">
        <DropdownMenuLabel className="grid gap-0.5 font-normal">
          <span className="truncate text-sm font-medium">{me.display_name}</span>
          <span className="truncate text-xs text-muted-foreground">{me.email}</span>
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem asChild>
          <Link to="/profile" onClick={onNavigate}>
            <UserRound /> {t("nav.profile")}
          </Link>
        </DropdownMenuItem>
        <DropdownMenuItem asChild>
          <Link to="/devices" onClick={onNavigate}>
            <Laptop /> {t("nav.devices")}
          </Link>
        </DropdownMenuItem>
        {notify && (
          <DropdownMenuItem asChild>
            <Link to="/notifications/settings" onClick={onNavigate}>
              <BellRing /> {t("nav.notifySettings")}
            </Link>
          </DropdownMenuItem>
        )}
        <DropdownMenuSeparator />
        <DropdownMenuSub>
          <DropdownMenuSubTrigger>
            <Sun className="dark:hidden" />
            <Moon className="hidden dark:block" /> {t("theme.label")}
          </DropdownMenuSubTrigger>
          <DropdownMenuSubContent>
            <DropdownMenuRadioGroup value={theme} onValueChange={(value) => setTheme(value as ThemeChoice)}>
              <DropdownMenuRadioItem value="light">
                <Sun /> {t("theme.light")}
              </DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="dark">
                <Moon /> {t("theme.dark")}
              </DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="system">
                <Monitor /> {t("theme.system")}
              </DropdownMenuRadioItem>
            </DropdownMenuRadioGroup>
          </DropdownMenuSubContent>
        </DropdownMenuSub>
        <DropdownMenuSub>
          <DropdownMenuSubTrigger>
            <Languages /> {t("language")}
          </DropdownMenuSubTrigger>
          <DropdownMenuSubContent>
            <DropdownMenuRadioGroup value={locale} onValueChange={(value) => setLocale(value as Locale)}>
              <DropdownMenuRadioItem value="th">ไทย</DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="en">English</DropdownMenuRadioItem>
            </DropdownMenuRadioGroup>
          </DropdownMenuSubContent>
        </DropdownMenuSub>
        <DropdownMenuSeparator />
        <DropdownMenuItem onSelect={() => void session.logout(false)}>
          <LogOut /> {t("signOut")}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

