import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FormError } from "../components/FormError";
import { useT, useCurrentLocale } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { formatDateTime } from "../lib/format";
import { useMe } from "../state/queries";
import { useSession } from "../state/session";

export function Devices() {
  const t = useT();
  const locale = useCurrentLocale();
  const session = useSession();
  const queryClient = useQueryClient();
  const me = useMe();
  const devices = useQuery({ queryKey: ["devices"], queryFn: async () => (await session.api.listDevices()).items });
  const revoke = useMutation({
    mutationFn: (id: string) => session.api.revokeDevice(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["devices"] }),
  });

  return (
    <section aria-labelledby="devices-heading">
      <h1 id="devices-heading">{t("devices.title")}</h1>
      {devices.isPending && <p role="status">{t("loading")}</p>}
      <FormError message={devices.isError ? t(errorKey(devices.error)) : revoke.isError ? t(errorKey(revoke.error)) : null} />
      {devices.data && (
        <ul className="rows">
          {devices.data.map((d) => (
            <li key={d.id}>
              <div>
                <strong>{d.name}</strong> <span className="muted">{d.platform}</span>
                {d.current && <span className="badge">{t("devices.current")}</span>}
                <br />
                <small className="muted">
                  {t("devices.lastSeen", { time: formatDateTime(d.last_seen_at, locale, me.data?.timezone) })}
                </small>
              </div>
              {!d.current && (
                <button
                  type="button"
                  disabled={revoke.isPending}
                  aria-label={t("devices.revokeNamed", { name: d.name })}
                  onClick={() => revoke.mutate(d.id)}
                >
                  {t("devices.revoke")}
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
      <p>
        <button type="button" onClick={() => void session.logout(true)}>
          {t("devices.signOutAll")}
        </button>
      </p>
    </section>
  );
}
