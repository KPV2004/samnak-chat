import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { MonitorSmartphone } from "lucide-react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { AccountLoadError } from "../components/AccountStates";
import { EmptyState } from "../components/EmptyState";
import { FormError } from "../components/FormError";
import { Page } from "../components/Page";
import { useT, useCurrentLocale } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { formatDateTime } from "../lib/format";
import { useMe } from "../state/queries";
import { useSession } from "../state/session";
import { DataList, DataRow, ListLoading } from "./module-ui";

export function Devices() {
  const t = useT();
  const locale = useCurrentLocale();
  const session = useSession();
  const queryClient = useQueryClient();
  const me = useMe();
  const devices = useQuery({ queryKey: ["devices"], queryFn: async () => (await session.api.listDevices()).items });
  const revoke = useMutation({
    mutationFn: (id: string) => session.api.revokeDevice(id),
    onSuccess: () => {
      toast.success(t("devices.revoked"));
      return queryClient.invalidateQueries({ queryKey: ["devices"] });
    },
  });

  return (
    <Page
      title={t("devices.title")}
      description={t("devices.body")}
      actions={
        <Button type="button" variant="outline" onClick={() => void session.logout(true)}>
          {t("devices.signOutAll")}
        </Button>
      }
    >
      {devices.isPending && <ListLoading />}
      {devices.isError ? (
        <AccountLoadError message={t(errorKey(devices.error))} onRetry={() => void devices.refetch()} />
      ) : (
        <FormError message={revoke.isError ? t(errorKey(revoke.error)) : null} />
      )}
      {devices.data &&
        (devices.data.length === 0 ? (
          <EmptyState icon={MonitorSmartphone} title={t("devices.none")} />
        ) : (
          <DataList>
            {devices.data.map((d) => (
              <DataRow key={d.id} className="flex-nowrap">
                <MonitorSmartphone className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
                <div className="grid min-w-0 flex-1 gap-0.5">
                  <p className="m-0 flex flex-wrap items-center gap-2 text-sm">
                    <span className="font-medium">{d.name}</span>
                    {d.current && <Badge variant="secondary">{t("devices.current")}</Badge>}
                  </p>
                  <p className="m-0 flex flex-wrap gap-x-2 text-xs text-muted-foreground">
                    <span>{d.platform}</span>
                    <span>
                      {t("devices.lastSeen", { time: formatDateTime(d.last_seen_at, locale, me.data?.timezone) })}
                    </span>
                  </p>
                </div>
                {!d.current && (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    disabled={revoke.isPending}
                    aria-label={t("devices.revokeNamed", { name: d.name })}
                    onClick={() => revoke.mutate(d.id)}
                  >
                    {t("devices.revoke")}
                  </Button>
                )}
              </DataRow>
            ))}
          </DataList>
        ))}
    </Page>
  );
}
