import { useLocale } from "../state/locale";
import { useT } from "../i18n/useT";
import type { Locale } from "../i18n/messages";

export function LanguageSelect() {
  const t = useT();
  const { locale, setLocale } = useLocale();
  return (
    <label className="inline">
      {t("language")}{" "}
      <select value={locale} onChange={(e) => setLocale(e.target.value as Locale)}>
        <option value="th">ไทย</option>
        <option value="en">English</option>
      </select>
    </label>
  );
}
