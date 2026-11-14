import { messages, type Locale, type MessageKey } from "./messages";

export function translate(locale: Locale, key: MessageKey): string {
  return messages[locale][key];
}

export function detectLocale(lang: string | undefined): Locale {
  return lang?.toLowerCase().startsWith("en") ? "en" : "th";
}
