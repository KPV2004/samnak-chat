import { useCallback } from "react";
import { translate, type Vars } from "./i18n";
import type { Locale, MessageKey } from "./messages";
import { useLocale } from "../state/locale";

export type T = (key: MessageKey, vars?: Vars) => string;

export function useT(): T {
  const locale = useLocale((s) => s.locale);
  // Stable between renders, so memoised components that take `t` or a callback built on it stay cached.
  return useCallback((key, vars) => translate(locale, key, vars), [locale]);
}

export function useCurrentLocale(): Locale {
  return useLocale((s) => s.locale);
}
