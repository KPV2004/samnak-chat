import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { applyTheme, initTheme, readStoredTheme, resolveTheme, themeStorageKey, useTheme } from "./theme";

/** A `matchMedia` that reports `dark` and lets the test flip it. */
function fakeMedia(dark: boolean) {
  const listeners = new Set<() => void>();
  const media = {
    matches: dark,
    addEventListener: (_: string, fn: () => void) => listeners.add(fn),
    removeEventListener: (_: string, fn: () => void) => listeners.delete(fn),
  };
  vi.stubGlobal("matchMedia", () => media);
  return {
    flip(next: boolean) {
      media.matches = next;
      listeners.forEach((fn) => fn());
    },
    listeners,
  };
}

beforeEach(() => {
  localStorage.clear();
  document.documentElement.classList.remove("dark");
  useTheme.setState({ theme: "system" });
});

afterEach(() => vi.unstubAllGlobals());

describe("theme", () => {
  it("follows the system unless a choice was stored", () => {
    expect(readStoredTheme()).toBe("system");
    expect(resolveTheme("system", true)).toBe("dark");
    expect(resolveTheme("system", false)).toBe("light");
    expect(resolveTheme("dark", false)).toBe("dark");
    expect(resolveTheme("light", true)).toBe("light");
  });

  it("ignores a stored value it does not know", () => {
    localStorage.setItem(themeStorageKey, "purple");
    expect(readStoredTheme()).toBe("system");
  });

  it("sets the dark class and colour scheme on the root element", () => {
    fakeMedia(false);
    applyTheme("dark");
    expect(document.documentElement).toHaveClass("dark");
    expect(document.documentElement.style.colorScheme).toBe("dark");
    applyTheme("light");
    expect(document.documentElement).not.toHaveClass("dark");
    expect(document.documentElement.style.colorScheme).toBe("light");
  });

  it("remembers the choice in localStorage and applies it", () => {
    fakeMedia(false);
    useTheme.getState().setTheme("dark");
    expect(localStorage.getItem(themeStorageKey)).toBe("dark");
    expect(document.documentElement).toHaveClass("dark");
    expect(readStoredTheme()).toBe("dark");
  });

  it("applies the stored choice on start and tracks the system while on system", () => {
    const media = fakeMedia(true);
    localStorage.setItem(themeStorageKey, "system");
    const stop = initTheme();
    expect(useTheme.getState().theme).toBe("system");
    expect(document.documentElement).toHaveClass("dark");
    media.flip(false);
    expect(document.documentElement).not.toHaveClass("dark");
    stop();
    expect(media.listeners.size).toBe(0);
  });

  it("does not follow the system once a theme is chosen", () => {
    const media = fakeMedia(false);
    localStorage.setItem(themeStorageKey, "light");
    initTheme();
    media.flip(true);
    expect(document.documentElement).not.toHaveClass("dark");
  });
});
