import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Link, useNavigate } from "react-router";
import { FormError } from "../components/FormError";
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

  async function join(id: string) {
    if (!chat) return;
    setJoining(id);
    setError(null);
    try {
      await chat.api.joinChannel(id);
      await chat.store.loadChannels();
      navigate(`/chat/${id}`);
    } catch (err) {
      setError(t(errorKey(err)));
      setJoining(null);
    }
  }

  return (
    <section aria-labelledby="browse-heading">
      <h1 id="browse-heading">{t("chat.browse.title")}</h1>
      <FormError message={error} />
      {list.isPending && <p role="status">{t("loading")}</p>}
      {list.isError && (
        <p role="alert">
          {t("chat.loadError")}{" "}
          <button type="button" onClick={() => void list.refetch()}>
            {t("retry")}
          </button>
        </p>
      )}
      {list.isSuccess && list.data.length === 0 && <p>{t("chat.browse.empty")}</p>}
      {list.isSuccess && list.data.length > 0 && (
        <ul className="rows" aria-label={t("chat.browse.title")}>
          {list.data.map((c) => (
            <li key={c.id}>
              <div>
                <strong>{c.name}</strong>
                {c.topic && <div className="muted">{c.topic}</div>}
                <small className="muted">{t("chat.members", { n: c.member_count })}</small>
              </div>
              {joined[c.id] ? (
                <span className="actions">
                  <span className="muted">{t("chat.browse.joined")}</span>
                  <Link to={`/chat/${c.id}`}>{t("chat.browse.open")}</Link>
                </span>
              ) : (
                <button
                  type="button"
                  aria-label={t("chat.browse.joinNamed", { name: c.name })}
                  disabled={joining === c.id}
                  onClick={() => void join(c.id)}
                >
                  {t("chat.browse.join")}
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
