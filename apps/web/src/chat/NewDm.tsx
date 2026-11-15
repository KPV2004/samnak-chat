import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { useNavigate } from "react-router";
import { FormError } from "../components/FormError";
import type { MessageKey } from "../i18n/messages";
import { useT } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { useChat } from "../state/chat";
import { MemberChoice } from "./MemberChoice";

const maxPeople = 8;

export function NewDm() {
  const t = useT();
  const chat = useChat();
  const navigate = useNavigate();
  const [people, setPeople] = useState<string[]>([]);
  const [problem, setProblem] = useState<MessageKey | null>(null);

  const open = useMutation({
    mutationFn: async () => {
      const channel = await chat!.api.openDm(people);
      await chat!.store.loadChannels();
      return channel;
    },
    onSuccess: (channel) => navigate(`/chat/${channel.id}`),
  });

  function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const next = people.length === 0 ? "chat.dm.none" : people.length > maxPeople ? "chat.dm.tooMany" : null;
    setProblem(next);
    if (!next) open.mutate();
  }

  return (
    <section aria-labelledby="new-dm-heading">
      <h1 id="new-dm-heading">{t("chat.dm.title")}</h1>
      <form onSubmit={onSubmit} noValidate>
        <MemberChoice
          legend={t("chat.dm.people")}
          hint={t("chat.dm.hint")}
          selected={people}
          onChange={setPeople}
          empty={t("chat.dm.empty")}
        />
        <FormError message={problem ? t(problem) : open.isError ? t(errorKey(open.error)) : null} />
        <button type="submit" disabled={open.isPending || !chat}>
          {t("chat.dm.start")}
        </button>
      </form>
    </section>
  );
}
