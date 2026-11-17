import { Languages } from "lucide-react";
import { NativeSelect } from "@/components/ui/native-select";
import { useLocale } from "../state/locale";
import { useT } from "../i18n/useT";
import type { Locale } from "../i18n/messages";

export function LanguageSelect() {
  const t = useT();
  const { locale, setLocale } = useLocale();
  return (
    <label className="relative inline-flex items-center">
      <span className="sr-only">{t("language")}</span>
      <Languages className="pointer-events-none absolute left-2 z-10 size-4 text-muted-foreground" aria-hidden="true" />
      <NativeSelect size="sm" className="pl-8" value={locale} onChange={(e) => setLocale(e.target.value as Locale)}>
        <option value="th">ไทย</option>
        <option value="en">English</option>
      </NativeSelect>
    </label>
  );
}
