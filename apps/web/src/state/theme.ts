import { create } from "zustand";

export type ThemeChoice = "system" | "light" | "dark";

export const themeStorageKey = "samnak.theme";
const choices: readonly ThemeChoice[] = ["system", "light", "dark"];
const darkQuery = "(prefers-color-scheme: dark)";

function systemMedia(): MediaQueryList | null {
  return typeof matchMedia === "function" ? matchMedia(darkQuery) : null;
}

function systemIsDark(): boolean {
  return systemMedia()?.matches ?? false;
}

/** The saved choice; anything missing, unknown or unreadable means "follow the system". */
export function readStoredTheme(): ThemeChoice {
  try {
    const stored = localStorage.getItem(themeStorageKey);
    return choices.find((c) => c === stored) ?? "system";
  } catch {
    return "system";
  }
}

export function resolveTheme(choice: ThemeChoice, systemDark: boolean): "light" | "dark" {
  return choice === "system" ? (systemDark ? "dark" : "light") : choice;
}

/** Puts the `dark` class (Tailwind's dark variant) and the matching `color-scheme` on `<html>`. */
export function applyTheme(choice: ThemeChoice) {
  const resolved = resolveTheme(choice, systemIsDark());
  const root = document.documentElement;
  root.classList.toggle("dark", resolved === "dark");
  root.style.colorScheme = resolved;
}

interface ThemeState {
  theme: ThemeChoice;
  /** What the operating system prefers; only matters while `theme` is "system". */
  systemDark: boolean;
  setTheme: (theme: ThemeChoice) => void;
}

export const useTheme = create<ThemeState>((set) => ({
  theme: readStoredTheme(),
  systemDark: systemIsDark(),
  setTheme: (theme) => {
    try {
      localStorage.setItem(themeStorageKey, theme);
    } catch {
      // Private mode: the choice lasts for this page only.
    }
    applyTheme(theme);
    set({ theme });
  },
}));

/** The theme that is showing right now, for libraries that need "light" or "dark" (the toaster). */
export function useResolvedTheme(): "light" | "dark" {
  return useTheme((s) => resolveTheme(s.theme, s.systemDark));
}

/** Applies the saved theme and follows the system while the choice is "system". Returns a cleanup. */
export function initTheme(): () => void {
  useTheme.setState({ theme: readStoredTheme(), systemDark: systemIsDark() });
  applyTheme(useTheme.getState().theme);
  const media = systemMedia();
  if (!media) return () => undefined;
  const onChange = () => {
    useTheme.setState({ systemDark: media.matches });
    applyTheme(useTheme.getState().theme);
  };
  media.addEventListener("change", onChange);
  return () => media.removeEventListener("change", onChange);
}
