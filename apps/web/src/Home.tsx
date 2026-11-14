import { useQuery } from "@tanstack/react-query";
import { api } from "./api";
import { translate } from "./i18n/i18n";
import type { Locale } from "./i18n/messages";
import { useLocale } from "./state/locale";

export function Home() {
  const { locale, setLocale } = useLocale();
  const t = (key: Parameters<typeof translate>[1]) => translate(locale, key);
  const meta = useQuery({ queryKey: ["meta"], queryFn: () => api.getMeta() });

  return (
    <main>
      <header>
        <h1>{t("title")}</h1>
        <label>
          {t("language")}{" "}
          <select value={locale} onChange={(e) => setLocale(e.target.value as Locale)}>
            <option value="th">ไทย</option>
            <option value="en">English</option>
          </select>
        </label>
      </header>
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
    </main>
  );
}
