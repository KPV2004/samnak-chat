import { messages, type Locale, type MessageKey } from "./messages";

export type Vars = Record<string, string | number>;

export function translate(locale: Locale, key: MessageKey, vars?: Vars): string {
  const text: string = messages[locale][key];
  return vars ? text.replace(/\{(\w+)\}/g, (m, name: string) => String(vars[name] ?? m)) : text;
}

export function detectLocale(lang: string | undefined): Locale {
  return lang?.toLowerCase().startsWith("en") ? "en" : "th";
}
