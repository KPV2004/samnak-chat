import { useQuery } from "@tanstack/react-query";
import { Hash } from "lucide-react";
import { useState } from "react";
import { Link, useNavigate } from "react-router";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { EmptyState } from "../components/EmptyState";
import { FormError } from "../components/FormError";
import { Page } from "../components/Page";
import { useT } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { useChat, useChatState } from "../state/chat";
import { useSessionState } from "../state/session";

export function BrowseChannels() {
  const t = useT();
  const chat = useChat();
  const navigate = useNavigate();
  const { workspaceId } = useSessionState();
  const { channels: joined } = useChatState();
  const [error, setError] = useState<string | null>(null);
  const [joining, setJoining] = useState<string | null>(null);

  const list = useQuery({
    queryKey: ["chat-public", workspaceId],
    enabled: chat !== null,
    staleTime: 0,
    queryFn: async () => (await chat!.api.listChannels("public")).items,
  });

  async function join(id: string, name: string) {
    if (!chat) return;
    setJoining(id);
    setError(null);
    try {
      await chat.api.joinChannel(id);
      await chat.store.loadChannels();
      toast.success(t("chat.toast.joined", { name }));
      navigate(`/chat/${id}`);
    } catch (err) {
      setError(t(errorKey(err)));
      setJoining(null);
    }
  }

  return (
    <Page title={t("chat.browse.title")}>
      <FormError message={error} />
      {list.isPending && (
        <div role="status" className="grid gap-3">
          <span className="sr-only">{t("loading")}</span>
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className="h-14 w-full rounded-lg" />
          ))}
        </div>
      )}
      {list.isError && (
        <div role="alert" className="flex flex-wrap items-center gap-3 text-sm">
          <span>{t("chat.loadError")}</span>
          <Button type="button" variant="outline" size="sm" onClick={() => void list.refetch()}>
            {t("retry")}
          </Button>
        </div>
      )}
      {list.isSuccess && list.data.length === 0 && (
        <EmptyState
          icon={Hash}
          title={t("chat.browse.empty")}
          action={
            <Button asChild variant="outline" size="sm">
              <Link to="/chat/new">{t("chat.newChannel")}</Link>
            </Button>
          }
        />
      )}
      {list.isSuccess && list.data.length > 0 && (
        <ul aria-label={t("chat.browse.title")} className="divide-y rounded-lg border">
          {list.data.map((c) => (
            <li key={c.id} className="flex flex-wrap items-center gap-3 px-4 py-3">
              <div className="grid min-w-0 flex-1 gap-0.5">
                <p className="m-0 font-medium">{c.name}</p>
                {c.topic && <p className="m-0 text-sm text-muted-foreground">{c.topic}</p>}
                <small className="text-xs text-muted-foreground">{t("chat.members", { n: c.member_count })}</small>
              </div>
              {joined[c.id] ? (
                <div className="flex items-center gap-2">
                  <Badge variant="secondary">{t("chat.browse.joined")}</Badge>
                  <Button asChild variant="outline" size="sm">
                    <Link to={`/chat/${c.id}`}>{t("chat.browse.open")}</Link>
                  </Button>
                </div>
              ) : (
                <Button
                  type="button"
                  size="sm"
                  aria-label={t("chat.browse.joinNamed", { name: c.name })}
                  disabled={joining === c.id}
                  onClick={() => void join(c.id, c.name)}
                >
                  {t("chat.browse.join")}
                </Button>
              )}
            </li>
          ))}
        </ul>
      )}
    </Page>
  );
}
