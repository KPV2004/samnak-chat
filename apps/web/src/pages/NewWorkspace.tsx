import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Loader2Icon } from "lucide-react";
import { useState } from "react";
import { Link, useNavigate } from "react-router";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardFooter } from "@/components/ui/card";
import { Field } from "../components/Field";
import { FormError } from "../components/FormError";
import { Page } from "../components/Page";
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
      toast.success(t("workspace.created"));
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
    <Page width="narrow" title={t("workspace.createTitle")} description={t("workspace.createBody")}>
      <form onSubmit={onSubmit} noValidate>
        <Card>
          <CardContent className="grid gap-4">
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
          </CardContent>
          <CardFooter className="justify-end gap-2">
            <Button variant="outline" asChild>
              <Link to="/">{t("cancel")}</Link>
            </Button>
            <Button type="submit" disabled={create.isPending}>
              {create.isPending && <Loader2Icon className="animate-spin" aria-hidden="true" />}
              {t("workspace.create")}
            </Button>
          </CardFooter>
        </Card>
      </form>
    </Page>
  );
}
