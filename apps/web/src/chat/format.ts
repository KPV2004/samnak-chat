export type Mark = "bold" | "italic" | "code" | "link";

/** The text after an edit, and what the box should select (a caret when both are equal). */
export interface Edit {
  text: string;
  start: number;
  end: number;
}

const marks: Record<Mark, [string, string]> = {
  bold: ["**", "**"],
  italic: ["_", "_"],
  code: ["`", "`"],
  link: ["[", "](https://)"],
};

/**
 * Puts a markdown mark round the selection (the subset `markdown.tsx` reads). With nothing selected
 * the caret lands between the marks. A link selects its address so it can be typed over.
 */
export function wrapSelection(text: string, start: number, end: number, mark: Mark): Edit {
  const [open, close] = marks[mark];
  const picked = text.slice(start, end);
  const next = text.slice(0, start) + open + picked + close + text.slice(end);
  if (mark === "link" && picked !== "") {
    const at = start + open.length + picked.length + 2;
    return { text: next, start: at, end: at + "https://".length };
  }
  const at = start + open.length;
  return { text: next, start: at, end: at + picked.length };
}

/** Puts `insert` at the selection, replacing it, with the caret after it. */
export function insertAt(text: string, start: number, end: number, insert: string): Edit {
  const at = start + insert.length;
  return { text: text.slice(0, start) + insert + text.slice(end), start: at, end: at };
}
