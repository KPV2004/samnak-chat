import type { NotifyPreferences } from "@samnak/core";
import { useState, type FormEvent } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { channelTitle, useNameOf } from "../chat/names";
import { AccountLoadError } from "../components/AccountStates";
import { Field } from "../components/Field";
import { FormError } from "../components/FormError";
import { Page } from "../components/Page";
import { RequireWorkspace } from "../components/RequireWorkspace";
import type { MessageKey } from "../i18n/messages";
import { useT } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { useEnabledModules } from "../modules";
import { preferenceKinds } from "../notify/kinds";
import { usePreferences, useSavePreferences } from "../notify/usePreferences";
import { useChat, useChatState } from "../state/chat";
import { ListLoading, Panel } from "./module-ui";

export function NotificationSettings() {
  const t = useT();
  return (
    <Page width="narrow" title={t("notify.prefs.title")} description={t("notify.prefs.body")}>
      <RequireWorkspace>{() => <SettingsLoader />}</RequireWorkspace>
    </Page>
  );
}

function SettingsLoader() {
  const t = useT();
  const prefs = usePreferences();
  if (prefs.isPending) {
    return <ListLoading rows={3} />;
  }
  if (prefs.isError) {
    return <AccountLoadError message={t(errorKey(prefs.error))} onRetry={() => void prefs.refetch()} />;
  }
  return <SettingsForm initial={prefs.data} />;
}

/** The form starts from the saved settings and keeps its own draft until Save. */
function SettingsForm({ initial }: { initial: NotifyPreferences }) {
  const t = useT();
  const enabledModules = useEnabledModules();
  const save = useSavePreferences();
  const latest = usePreferences().data ?? initial;
  // Kinds that are off. Kinds of a module that is off stay in the set, so saving keeps them.
  const [off, setOff] = useState(() => new Set(initial.disabled_kinds));
  const [from, setFrom] = useState(initial.dnd_start ?? "");
  const [until, setUntil] = useState(initial.dnd_end ?? "");
  const [problem, setProblem] = useState<MessageKey | null>(null);
  const kinds = preferenceKinds.filter((k) => enabledModules.includes(k.module));

  function setKindOn(kind: string, on: boolean) {
    setOff((prev) => {
      const next = new Set(prev);
      if (on) next.delete(kind);
      else next.add(kind);
      return next;
    });
  }

  function submit(e: FormEvent) {
    e.preventDefault();
    if ((from === "") !== (until === "")) return setProblem("notify.prefs.dndBoth");
    if (from !== "" && from === until) return setProblem("notify.prefs.dndSame");
    setProblem(null);
    save.mutate(
      {
        disabled_kinds: [...off],
        // Muted channels are changed from the header and this list at once, so the saved list is the one to keep.
        muted_channel_ids: latest.muted_channel_ids,
        dnd_start: from || null,
        dnd_end: until || null,
      },
      { onSuccess: () => toast.success(t("notify.prefs.saved")) },
    );
  }

  return (
    <form onSubmit={submit} className="grid gap-6">
      <FormError message={problem ? t(problem) : save.isError ? t(errorKey(save.error)) : null} />

      <Panel title={t("notify.prefs.kinds")} bodyClassName="gap-3">
        <p className="m-0 text-sm text-muted-foreground">{t("notify.prefs.kindsHint")}</p>
        {kinds.map((k) => (
          <div key={k.kind} className="flex items-center gap-2">
            <Checkbox
              id={`notify-kind-${k.kind}`}
              checked={!off.has(k.kind)}
              onCheckedChange={(on) => setKindOn(k.kind, on === true)}
            />
            <Label htmlFor={`notify-kind-${k.kind}`} className="font-normal">
              {t(k.label)}
            </Label>
          </div>
        ))}
      </Panel>

      <Panel title={t("notify.prefs.dnd")} bodyClassName="gap-3">
        <p className="m-0 text-sm text-muted-foreground">{t("notify.prefs.dndBody")}</p>
        <div className="grid grid-cols-2 gap-3 sm:max-w-sm">
          <Field
            label={t("notify.prefs.dndStart")}
            type="time"
            value={from}
            onChange={(e) => setFrom(e.target.value)}
          />
          <Field
            label={t("notify.prefs.dndEnd")}
            type="time"
            value={until}
            onChange={(e) => setUntil(e.target.value)}
          />
        </div>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="justify-self-start"
          onClick={() => {
            setFrom("");
            setUntil("");
          }}
        >
          {t("notify.prefs.dndClear")}
        </Button>
      </Panel>

      <MutedChannels current={latest} />

      <div className="flex justify-end">
        <Button type="submit" disabled={save.isPending}>
          {t("save")}
        </Button>
      </div>
    </form>
  );
}

/** Unmuting saves at once, from the latest saved settings; a failure shows in the form's error line. */
function MutedChannels({ current }: { current: NotifyPreferences }) {
  const t = useT();
  const save = useSavePreferences();
  const chat = useChat();
  const { channels } = useChatState();
  const nameOf = useNameOf();

  const titleOf = (channelId: string) => {
    const channel = channels[channelId]?.channel;
    return channel ? channelTitle(channel, chat?.userId ?? "", nameOf, t) : t("notify.prefs.unknownChannel");
  };

  function unmute(channelId: string) {
    save.mutate(
      { ...current, muted_channel_ids: current.muted_channel_ids.filter((id) => id !== channelId) },
      { onSuccess: () => toast.success(t("notify.unmuted")) },
    );
  }

  return (
    <Panel title={t("notify.prefs.muted")} bodyClassName="gap-3">
      <p className="m-0 text-sm text-muted-foreground">{t("notify.prefs.mutedBody")}</p>
      {current.muted_channel_ids.length === 0 ? (
        <p className="m-0 text-sm text-muted-foreground">{t("notify.prefs.noMuted")}</p>
      ) : (
        <ul aria-label={t("notify.prefs.muted")} className="m-0 list-none divide-y p-0">
          {current.muted_channel_ids.map((id) => {
            const name = titleOf(id);
            return (
              <li key={id} className="flex items-center gap-2 py-2">
                <span className="min-w-0 flex-1 break-words">{name}</span>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={save.isPending}
                  aria-label={t("notify.prefs.unmuteNamed", { name })}
                  onClick={() => unmute(id)}
                >
                  {t("notify.unmute")}
                </Button>
              </li>
            );
          })}
        </ul>
      )}
    </Panel>
  );
}
