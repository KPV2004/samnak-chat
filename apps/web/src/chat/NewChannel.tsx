import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { useNavigate } from "react-router";
import { Field } from "../components/Field";
import { FormError } from "../components/FormError";
import { useT } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { useChat } from "../state/chat";
import { MemberChoice } from "./MemberChoice";

export function NewChannel() {
  const t = useT();
  const chat = useChat();
  const navigate = useNavigate();
  const [name, setName] = useState("");
  const [topic, setTopic] = useState("");
  const [type, setType] = useState<"public" | "private">("public");
  const [people, setPeople] = useState<string[]>([]);
  const [nameError, setNameError] = useState(false);

  const create = useMutation({
    mutationFn: async () => {
      const channel = await chat!.api.createChannel({
        type,
        name: name.trim(),
        ...(topic.trim() ? { topic: topic.trim() } : {}),
        ...(people.length > 0 ? { member_ids: people } : {}),
      });
      await chat!.store.loadChannels();
      return channel;
    },
    onSuccess: (channel) => navigate(`/chat/${channel.id}`),
  });

  function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const empty = !name.trim();
    setNameError(empty);
    if (empty) {
      (e.currentTarget.elements.namedItem("name") as HTMLElement).focus();
      return;
    }
    create.mutate();
  }

  return (
    <section aria-labelledby="new-channel-heading">
      <h1 id="new-channel-heading">{t("chat.new.title")}</h1>
      <form onSubmit={onSubmit} noValidate>
        <Field
          label={t("chat.new.name")}
          name="name"
          value={name}
          maxLength={80}
          onChange={(e) => setName(e.target.value)}
          hint={t("chat.new.nameHint")}
          error={nameError ? t("chat.new.nameRequired") : undefined}
        />
        <Field
          label={t("chat.new.topic")}
          name="topic"
          value={topic}
          maxLength={250}
          onChange={(e) => setTopic(e.target.value)}
        />
        <fieldset>
          <legend>{t("chat.new.visibility")}</legend>
          <label>
            <input type="radio" name="type" checked={type === "public"} onChange={() => setType("public")} />{" "}
            {t("chat.new.public")}
          </label>
          <br />
          <label>
            <input type="radio" name="type" checked={type === "private"} onChange={() => setType("private")} />{" "}
            {t("chat.new.private")}
          </label>
        </fieldset>
        <MemberChoice legend={t("chat.new.people")} selected={people} onChange={setPeople} empty={t("chat.dm.empty")} />
        <FormError message={create.isError ? t(errorKey(create.error, { 409: "chat.new.nameTaken" })) : null} />
        <button type="submit" disabled={create.isPending || !chat}>
          {t("chat.new.create")}
        </button>
      </form>
    </section>
  );
}
