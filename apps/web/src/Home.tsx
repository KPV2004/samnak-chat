import { useQuery } from "@tanstack/react-query";
import { Inbox } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { AccountLoadError, AccountLoading } from "./components/AccountStates";
import { EmptyState } from "./components/EmptyState";
import { Page } from "./components/Page";
import { api } from "./api";
import { useT } from "./i18n/useT";
import { useWorkspaces } from "./state/queries";

export function Home() {
  const t = useT();
  const { current } = useWorkspaces();
  const meta = useQuery({ queryKey: ["meta"], queryFn: () => api.getMeta() });

  return (
    <Page
      title={t("title")}
      description={
        current && (
          <>
            {t("workspace.current")} <strong className="font-medium text-foreground">{current.name}</strong>
          </>
        )
      }
    >
      <section aria-labelledby="modules-heading" className="grid gap-3">
        <h2 id="modules-heading" className="text-base font-semibold">
          {t("modules")}
        </h2>
        {meta.isPending && (
          <AccountLoading>
            <div className="flex gap-2">
              <Skeleton className="h-6 w-16 rounded-full" />
              <Skeleton className="h-6 w-20 rounded-full" />
            </div>
          </AccountLoading>
        )}
        {meta.isError && <AccountLoadError message={t("error")} onRetry={() => void meta.refetch()} />}
        {meta.isSuccess &&
          (meta.data.modules.length === 0 ? (
            <EmptyState icon={Inbox} title={t("noModules")} />
          ) : (
            <ul className="flex flex-wrap gap-2">
              {meta.data.modules.map((m) => (
                <li key={m}>
                  <Badge variant="secondary">{m}</Badge>
                </li>
              ))}
            </ul>
          ))}
      </section>
    </Page>
  );
}
