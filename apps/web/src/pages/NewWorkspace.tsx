import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useNavigate } from "react-router";
import { Field } from "../components/Field";
import { FormError } from "../components/FormError";
import type { MessageKey } from "../i18n/messages";
import { useT } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { slugify } from "../lib/format";
import { useSession } from "../state/session";

const slugPattern = /^[a-z0-9-]{3,40}$/;

export function NewWorkspace() {
  const t = useT();
  const session = useSession();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [slugTouched, setSlugTouched] = useState(false);
  const [errors, setErrors] = useState<Partial<Record<"name" | "slug", MessageKey>>>({});

  const create = useMutation({
    mutationFn: () => session.api.createWorkspace({ name: name.trim(), slug }),
    onSuccess: (ws) => {
      queryClient.setQueryData<unknown[]>(["workspaces"], (old) => [...(old ?? []), ws]);
      session.setWorkspaceId(ws.id);
      navigate("/");
    },
  });

  function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const next: typeof errors = {};
    if (!name.trim()) next.name = "workspace.nameRequired";
    if (!slugPattern.test(slug)) next.slug = "workspace.slugInvalid";
    setErrors(next);
    const first = (["name", "slug"] as const).find((k) => next[k]);
    if (first) return (form.elements.namedItem(first) as HTMLElement).focus();
    create.mutate();
  }

  return (
    <section aria-labelledby="new-ws-heading">
      <h1 id="new-ws-heading">{t("workspace.createTitle")}</h1>
      <form onSubmit={onSubmit} noValidate>
        <Field
          label={t("workspace.name")}
          name="name"
          value={name}
          onChange={(e) => {
            setName(e.target.value);
            if (!slugTouched) setSlug(slugify(e.target.value));
          }}
          error={errors.name && t(errors.name)}
        />
        <Field
          label={t("workspace.slug")}
          name="slug"
          value={slug}
          onChange={(e) => {
            setSlugTouched(true);
            setSlug(e.target.value);
          }}
          hint={t("workspace.slugHint")}
          error={errors.slug && t(errors.slug)}
        />
        <FormError message={create.isError ? t(errorKey(create.error, { 409: "workspace.slugTaken" })) : null} />
        <button type="submit" disabled={create.isPending}>
          {t("workspace.create")}
        </button>
      </form>
    </section>
  );
}
