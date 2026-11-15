import { useQuery } from "@tanstack/react-query";
import { api } from "./api";
import { useT } from "./i18n/useT";
import { useWorkspaces } from "./state/queries";

export function Home() {
  const t = useT();
  const { current } = useWorkspaces();
  const meta = useQuery({ queryKey: ["meta"], queryFn: () => api.getMeta() });

  return (
    <>
      <h1>{t("title")}</h1>
      {current && (
        <p>
          {t("workspace.current")} <strong>{current.name}</strong>
        </p>
      )}
      <section aria-labelledby="modules-heading">
        <h2 id="modules-heading">{t("modules")}</h2>
        {meta.isPending && <p role="status">{t("loading")}</p>}
        {meta.isError && (
          <p role="alert">
            {t("error")}{" "}
            <button type="button" onClick={() => void meta.refetch()}>
              {t("retry")}
            </button>
          </p>
        )}
        {meta.isSuccess &&
          (meta.data.modules.length === 0 ? (
            <p>{t("noModules")}</p>
          ) : (
            <ul>
              {meta.data.modules.map((m) => (
                <li key={m}>{m}</li>
              ))}
            </ul>
          ))}
      </section>
    </>
  );
}
