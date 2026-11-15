import { translate, type Vars } from "./i18n";
import type { Locale, MessageKey } from "./messages";
import { useLocale } from "../state/locale";

export type T = (key: MessageKey, vars?: Vars) => string;

export function useT(): T {
  const locale = useLocale((s) => s.locale);
  return (key, vars) => translate(locale, key, vars);
}

export function useCurrentLocale(): Locale {
  return useLocale((s) => s.locale);
}
