/** An `@` the caret is in, and the letters typed after it. */
export interface MentionQuery {
  /** Index of the `@`. */
  start: number;
  query: string;
}

/**
 * The mention being typed at `caret`: an `@` at the start of the text or after
 * whitespace, followed by non-space characters. An `@` inside a word (an e-mail
 * address) is not a mention.
 */
export function mentionAt(text: string, caret: number): MentionQuery | null {
  const before = text.slice(0, caret);
  const start = before.lastIndexOf("@");
  if (start < 0) return null;
  if (start > 0 && !/\s/.test(before[start - 1]!)) return null;
  const query = before.slice(start + 1);
  return /\s/.test(query) ? null : { start, query };
}

export interface MentionOption {
  /** What is inserted: `<@user_id>` or `<!channel>`. */
  token: string;
  label: string;
  /** Extra description, e.g. for @channel. */
  detail?: string;
}

/** Replaces the typed `@query` with `token` and a space; returns the new text and caret. */
export function insertMention(text: string, caret: number, at: MentionQuery, token: string) {
  const next = `${text.slice(0, at.start)}${token} ${text.slice(caret)}`;
  return { text: next, caret: at.start + token.length + 1 };
}
