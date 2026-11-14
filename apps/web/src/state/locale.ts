import { create } from "zustand";
import { detectLocale } from "../i18n/i18n";
import type { Locale } from "../i18n/messages";

interface LocaleState {
  locale: Locale;
  setLocale: (locale: Locale) => void;
}

export const useLocale = create<LocaleState>((set) => ({
  locale: detectLocale(typeof navigator === "undefined" ? undefined : navigator.language),
  setLocale: (locale) => set({ locale }),
}));
