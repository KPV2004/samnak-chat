/*
 * The colours of avatars that have no picture. Six fills, picked from an id so a person or a chat
 * keeps its colour on every screen and in every tab. The hues are the ones index.css already uses
 * (green 151, teal 200, lime 120, amber 80, red 25, and the neutral green-grey); there is no
 * violet. The lightness is the same in light and dark mode, so every fill carries white text.
 */
export const avatarColors = [
  "bg-[oklch(0.46_0.108_151)]",
  "bg-[oklch(0.5_0.1_200)]",
  "bg-[oklch(0.5_0.11_120)]",
  "bg-[oklch(0.5_0.1_80)]",
  "bg-[oklch(0.52_0.14_25)]",
  "bg-[oklch(0.45_0.02_151)]",
] as const;

/** FNV-1a over the id: cheap, stable, and well spread over short and long ids alike. */
function hash(id: string): number {
  let h = 0x811c9dc5;
  for (let i = 0; i < id.length; i++) {
    h ^= id.charCodeAt(i);
    h = Math.imul(h, 0x01000193);
  }
  return h >>> 0;
}

/** The fill class for an id. */
export function avatarColorFor(id: string): (typeof avatarColors)[number] {
  return avatarColors[hash(id) % avatarColors.length]!;
}
