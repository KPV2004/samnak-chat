import { useSyncExternalStore } from "react";

const query = "(min-width: 64rem)";

/**
 * Whether the screen is at least `lg` wide, wide enough to hold the thread beside the channel (the
 * sidebar and the rail take 312px of it). Without matchMedia (jsdom) the screen counts as wide, so the
 * thread panel is a side panel there.
 */
export function useWide(): boolean {
  return useSyncExternalStore(
    (onChange) => {
      const media = window.matchMedia?.(query);
      media?.addEventListener("change", onChange);
      return () => media?.removeEventListener("change", onChange);
    },
    () => window.matchMedia?.(query).matches ?? true,
    () => true,
  );
}
