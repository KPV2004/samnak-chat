import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { useNavigate } from "react-router";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Field } from "../components/Field";
import { FormError } from "../components/FormError";
import { Page } from "../components/Page";
import { useT } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { useChat } from "../state/chat";
import { MoreOptions } from "../components/MoreOptions";
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
    onSuccess: (channel) => {
      toast.success(t("chat.toast.created", { name: channel.name }));
      navigate(`/chat/${channel.id}`);
    },
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
    <Page title={t("chat.new.title")} width="narrow">
      <form onSubmit={onSubmit} noValidate className="grid gap-4">
        <Field
          label={t("chat.new.name")}
          name="name"
          value={name}
          maxLength={80}
          onChange={(e) => setName(e.target.value)}
          hint={t("chat.new.nameHint")}
          error={nameError ? t("chat.new.nameRequired") : undefined}
        />
        <fieldset className="grid gap-2">
          <legend className="text-sm font-medium">{t("chat.new.visibility")}</legend>
          {(["public", "private"] as const).map((option) => (
            <label
              key={option}
              className="flex cursor-pointer items-start gap-3 rounded-lg border px-3 py-2.5 text-sm has-[:checked]:border-primary has-[:checked]:bg-primary/5"
            >
              <input
                type="radio"
                name="type"
                className="mt-0.5 size-4 accent-primary"
                checked={type === option}
                onChange={() => setType(option)}
              />
              <span>{option === "public" ? t("chat.new.public") : t("chat.new.private")}</span>
            </label>
          ))}
        </fieldset>
        <MoreOptions>
          <Field
            label={t("chat.new.topic")}
            name="topic"
            value={topic}
            maxLength={250}
            onChange={(e) => setTopic(e.target.value)}
          />
          <MemberChoice legend={t("chat.new.people")} selected={people} onChange={setPeople} empty={t("chat.dm.empty")} />
        </MoreOptions>
        <FormError message={create.isError ? t(errorKey(create.error, { 409: "chat.new.nameTaken" })) : null} />
        <div>
          <Button type="submit" disabled={create.isPending || !chat}>
            {t("chat.new.create")}
          </Button>
        </div>
      </form>
    </Page>
  );
}
