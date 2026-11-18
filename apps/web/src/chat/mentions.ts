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
  /** What is sent: `<@user_id>` or `<!channel>`. */
  token: string;
  label: string;
  /** Extra description, e.g. for @channel. */
  detail?: string;
  /** What the box shows instead of the token: `@Somchai`. */
  insert?: string;
}

/** Replaces the typed `@query` with `insert` and a space; returns the new text and caret. */
export function insertMention(text: string, caret: number, at: MentionQuery, insert: string) {
  const next = `${text.slice(0, at.start)}${insert} ${text.slice(caret)}`;
  return { text: next, caret: at.start + insert.length + 1 };
}

/** The mentions of a draft: what the box shows (`@Somchai`) -> the token that is sent (`<@u2>`). */
export type Picks = Map<string, string>;

/** What the box shows for `<!channel>`. */
export const channelText = "@channel";

const tokenPattern = /<@([^>\s]+)>|<!channel>/g;

/**
 * The text a body is shown as: each `<@user_id>` becomes `@name`, and `<!channel>` becomes
 * `@channel`. The picks turn those names back into the tokens when the text is sent.
 * Two people with the same name cannot both be a name, so their tokens stay as they are.
 */
export function toReadable(body: string, nameOf: (userId: string) => string): { text: string; picks: Picks } {
  const shownOf = (userId: string | undefined) => (userId === undefined ? channelText : `@${nameOf(userId)}`);
  // The token behind each name; null when two different tokens share it.
  const tokens = new Map<string, string | null>();
  for (const [token, userId] of body.matchAll(tokenPattern)) {
    const shown = shownOf(userId);
    const known = tokens.get(shown);
    tokens.set(shown, known === undefined || known === token ? token : null);
  }
  const picks: Picks = new Map();
  for (const [shown, token] of tokens) {
    if (token !== null) picks.set(shown, token);
  }
  const text = body.replace(tokenPattern, (token: string, userId: string | undefined) => {
    const shown = shownOf(userId);
    return tokens.get(shown) === null ? token : shown;
  });
  return { text, picks };
}

/**
 * The text to show for a picked member, and the picks after it. A name that another person in
 * the draft already has is not taken: the token goes in as it is, so it is sent to the right person.
 */
export function addPick(picks: Picks, insert: string, token: string): { shown: string; picks: Picks } {
  const holder = picks.get(insert);
  if (holder !== undefined && holder !== token) return { shown: token, picks };
  return { shown: insert, picks: new Map(picks).set(insert, token) };
}

// A letter, digit or mark (Thai vowels and tone marks are marks) or underscore.
const wordChar = "[\\p{L}\\p{N}\\p{M}_]";

/**
 * The body to send: each picked name in `text` becomes its token. Only whole names match, so
 * `@tt` does not change `@tta`, and an e-mail address does not change. The longest name wins
 * when one starts with another.
 */
export function toTokens(text: string, picks: Picks): string {
  if (picks.size === 0) return text;
  const names = [...picks.keys()].sort((a, b) => b.length - a.length).map(escapeRegExp);
  const pattern = new RegExp(`(?<!${wordChar})(${names.join("|")})(?!${wordChar})`, "gu");
  return text.replace(pattern, (name) => picks.get(name)!);
}

function escapeRegExp(text: string): string {
  return text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
