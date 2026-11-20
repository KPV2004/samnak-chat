import type { HelpdeskQueue } from "@samnak/core";
import { createM2Api } from "@samnak/core";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Archive, Inbox, Plus } from "lucide-react";
import { useMemo, useState, type FormEvent } from "react";
import { Navigate } from "react-router";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/native-select";
import { AccountLoadError } from "../components/AccountStates";
import { EmptyState } from "../components/EmptyState";
import { Field } from "../components/Field";
import { FormError } from "../components/FormError";
import { Page } from "../components/Page";
import { RequireWorkspace } from "../components/RequireWorkspace";
import { useT } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { ListLoading, Panel } from "../pages/module-ui";
import { canManage, useWorkspaces } from "../state/queries";
import { useSession, useSessionState } from "../state/session";
import { useHelpdeskApi } from "./api";
import { useMembers } from "./useMembers";

const prefixPattern = /^[A-Za-z]{2,6}$/;

/** Queues, their agents, channels and categories (HD-01, HD-02). Owners and admins only. */
export function Settings() {
  return <RequireWorkspace>{() => <SettingsView />}</RequireWorkspace>;
}

function SettingsView() {
  const t = useT();
  const api = useHelpdeskApi();
  const session = useSession();
  const { workspaceId } = useSessionState();
  const workspace = useWorkspaces().current;
  const queues = useQuery({
    queryKey: ["helpdesk", "queues", workspaceId],
    queryFn: () => api.listQueues(),
    enabled: workspaceId !== null,
  });
  const m2 = useMemo(() => createM2Api(session.client), [session]);
  const channels = useQuery({
    queryKey: ["channels", workspaceId, "all"],
    queryFn: () => m2.listChannels(),
    enabled: workspaceId !== null,
  });

  if (!canManage(workspace?.role)) return <Navigate to="/helpdesk" replace />;

  const items = queues.data?.items ?? [];
  return (
    <Page title={t("helpdesk.settings")} description={t("helpdesk.settings.body")} width="wide">
      <NewQueue />
      {queues.isPending && <ListLoading rows={2} />}
      {queues.isError && <AccountLoadError message={t("helpdesk.loadError")} onRetry={() => void queues.refetch()} />}
      {queues.isSuccess && items.length === 0 && <EmptyState icon={Inbox} title={t("helpdesk.settings.noQueues")} />}
      {items.map((queue) => (
        <QueueCard key={queue.id} queue={queue} channels={channels.data?.items ?? []} />
      ))}
    </Page>
  );
}

function NewQueue() {
  const t = useT();
  const api = useHelpdeskApi();
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [prefix, setPrefix] = useState("");
  const [nameMissing, setNameMissing] = useState(false);
  const [prefixBad, setPrefixBad] = useState(false);
  const create = useMutation({
    mutationFn: () => api.createQueue({ name: name.trim(), prefix: prefix.trim().toUpperCase() }),
    onSuccess: () => {
      setName("");
      setPrefix("");
      setNameMissing(false);
      setPrefixBad(false);
      void queryClient.invalidateQueries({ queryKey: ["helpdesk", "queues"] });
    },
  });
  const conflict = create.error ? errorKey(create.error, { 409: "helpdesk.error.prefixTaken" }) : null;

  function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const noName = name.trim() === "";
    const badPrefix = !prefixPattern.test(prefix.trim());
    setNameMissing(noName);
    setPrefixBad(badPrefix);
    if (noName || badPrefix) return;
    create.mutate();
  }

  return (
    <Panel title={t("helpdesk.settings.newQueue")}>
      <form onSubmit={submit} noValidate className="grid gap-4 sm:grid-cols-2">
        <Field
          label={t("helpdesk.field.name")}
          error={nameMissing ? t("helpdesk.field.nameRequired") : undefined}
          value={name}
          maxLength={100}
          onChange={(e) => setName(e.target.value)}
        />
        <Field
          label={t("helpdesk.field.prefix")}
          hint={t("helpdesk.field.prefixHint")}
          error={prefixBad ? t("helpdesk.field.prefixInvalid") : undefined}
          value={prefix}
          maxLength={6}
          onChange={(e) => setPrefix(e.target.value)}
        />
        <FormError message={conflict ? t(conflict) : null} />
        <div className="sm:col-span-2">
          <Button type="submit" disabled={create.isPending}>
            <Plus />
            {t("helpdesk.settings.createQueue")}
          </Button>
        </div>
      </form>
    </Panel>
  );
}

function QueueCard({ queue, channels }: { queue: HelpdeskQueue; channels: { id: string; name: string }[] }) {
  const t = useT();
  const api = useHelpdeskApi();
  const queryClient = useQueryClient();
  const members = useMembers();
  const [name, setName] = useState(queue.name);
  const [channelId, setChannelId] = useState(queue.channel_id ?? "");
  const [agents, setAgents] = useState<string[]>(queue.agent_ids);
  const [archiving, setArchiving] = useState(false);

  const refresh = () => queryClient.invalidateQueries({ queryKey: ["helpdesk", "queues"] });
  const save = useMutation({
    mutationFn: () =>
      api.updateQueue(queue.id, {
        ...(name.trim() !== queue.name && name.trim() !== "" ? { name: name.trim() } : {}),
        ...(channelId !== (queue.channel_id ?? "") ? { channel_id: channelId === "" ? null : channelId } : {}),
      }),
    onSuccess: refresh,
  });
  const saveAgents = useMutation({
    mutationFn: () => api.setQueueAgents(queue.id, agents),
    onSuccess: refresh,
  });
  const archive = useMutation({
    mutationFn: () => api.updateQueue(queue.id, { archived: true }),
    onSuccess: () => {
      setArchiving(false);
      return refresh();
    },
  });

  function toggleAgent(id: string) {
    setAgents((current) => (current.includes(id) ? current.filter((x) => x !== id) : [...current, id]));
  }

  return (
    <Panel
      title={queue.name}
      bodyClassName="gap-5"
      actions={
        <Button type="button" variant="outline" size="sm" onClick={() => setArchiving(true)}>
          <Archive />
          {t("helpdesk.settings.archive")}
        </Button>
      }
    >
      <p className="m-0 flex items-center gap-2 text-xs text-muted-foreground">
        <Badge variant="outline">{queue.prefix}</Badge>
        {t("helpdesk.settings.categoriesHint")}
      </p>
      <form
        className="grid gap-4 sm:grid-cols-2"
        onSubmit={(e) => {
          e.preventDefault();
          save.mutate();
        }}
      >
        <Field
          label={t("helpdesk.field.name")}
          value={name}
          maxLength={100}
          onChange={(e) => setName(e.target.value)}
        />
        <Field label={t("helpdesk.field.channel")} hint={t("helpdesk.field.channelHint")}>
          {({ id, describedBy }) => (
            <NativeSelect
              id={id}
              aria-describedby={describedBy}
              containerClassName="w-full"
              value={channelId}
              onChange={(e) => setChannelId(e.target.value)}
            >
              <option value="">{t("helpdesk.field.channelNone")}</option>
              {channels.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
            </NativeSelect>
          )}
        </Field>
        <div className="flex items-end">
          <Button type="submit" disabled={save.isPending}>
            {t("helpdesk.settings.saveQueue")}
          </Button>
        </div>
        <FormError message={save.error ? t(errorKey(save.error)) : null} />
      </form>

      <section className="grid gap-2">
        <h3 className="m-0 text-sm font-semibold">{t("helpdesk.settings.agents")}</h3>
        <p className="m-0 text-xs text-muted-foreground">{t("helpdesk.settings.agentsHint")}</p>
        {(members.data ?? []).map((m) => (
          <label key={m.user_id} className="flex items-center gap-2 text-sm">
            <Checkbox checked={agents.includes(m.user_id)} onCheckedChange={() => toggleAgent(m.user_id)} />
            {m.display_name || m.email}
          </label>
        ))}
        {(members.data ?? []).length === 0 && (
          <p className="m-0 text-sm text-muted-foreground">{t("helpdesk.noAgents")}</p>
        )}
        <div>
          <Button type="button" variant="outline" disabled={saveAgents.isPending} onClick={() => saveAgents.mutate()}>
            {t("helpdesk.settings.saveAgents")}
          </Button>
        </div>
      </section>

      <Categories queue={queue} onChanged={refresh} />

      <Dialog open={archiving} onOpenChange={setArchiving}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("helpdesk.settings.archiveTitle", { name: queue.name })}</DialogTitle>
            <DialogDescription>{t("helpdesk.settings.archiveHint")}</DialogDescription>
          </DialogHeader>
          <FormError message={archive.error ? t(errorKey(archive.error)) : null} />
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setArchiving(false)}>
              {t("cancel")}
            </Button>
            <Button type="button" variant="destructive" disabled={archive.isPending} onClick={() => archive.mutate()}>
              {t("helpdesk.settings.archiveConfirm")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Panel>
  );
}

function Categories({ queue, onChanged }: { queue: HelpdeskQueue; onChanged: () => void }) {
  const t = useT();
  const api = useHelpdeskApi();
  const [name, setName] = useState("");
  const create = useMutation({
    mutationFn: () => api.createCategory({ queue_id: queue.id, name: name.trim() }),
    onSuccess: () => {
      setName("");
      onChanged();
    },
  });
  const update = useMutation({
    mutationFn: (input: { id: string; is_default?: boolean; archived?: boolean }) =>
      api.updateCategory(input.id, {
        ...(input.is_default !== undefined ? { is_default: input.is_default } : {}),
        ...(input.archived !== undefined ? { archived: input.archived } : {}),
      }),
    onSuccess: onChanged,
  });

  return (
    <section className="grid gap-2">
      <h3 className="m-0 text-sm font-semibold">{t("helpdesk.settings.categories")}</h3>
      {queue.categories.length === 0 ? (
        <p className="m-0 text-sm text-muted-foreground">{t("helpdesk.settings.noCategories")}</p>
      ) : (
        <ul className="m-0 grid list-none gap-1 p-0">
          {queue.categories.map((c) => (
            <li key={c.id} className="flex flex-wrap items-center gap-2 text-sm">
              <span>{c.name}</span>
              {c.is_default ? (
                <Badge variant="secondary">{t("helpdesk.settings.default")}</Badge>
              ) : (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  disabled={update.isPending}
                  onClick={() => update.mutate({ id: c.id, is_default: true })}
                >
                  {t("helpdesk.settings.makeDefault")}
                </Button>
              )}
              <Button
                type="button"
                variant="ghost"
                size="sm"
                aria-label={t("helpdesk.settings.archiveCategory", { name: c.name })}
                disabled={update.isPending}
                onClick={() => update.mutate({ id: c.id, archived: true })}
              >
                <Archive />
              </Button>
            </li>
          ))}
        </ul>
      )}
      <form
        className="flex flex-wrap items-end gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          if (name.trim() !== "") create.mutate();
        }}
      >
        <Label htmlFor={`new-category-${queue.id}`} className="sr-only">
          {t("helpdesk.settings.newCategory")}
        </Label>
        <Input
          id={`new-category-${queue.id}`}
          className="w-56"
          placeholder={t("helpdesk.settings.newCategory")}
          aria-label={t("helpdesk.settings.newCategory")}
          maxLength={100}
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
        <Button type="submit" variant="outline" disabled={create.isPending || name.trim() === ""}>
          {t("helpdesk.settings.addCategory")}
        </Button>
      </form>
      <FormError message={create.error ? t(errorKey(create.error)) : null} />
    </section>
  );
}
