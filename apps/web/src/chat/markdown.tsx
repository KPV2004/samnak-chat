import { Fragment, type ReactNode } from "react";

// A small subset: **bold**, *italic* or _italic_, `code`, [text](url), bare
// http(s) URLs, <@user_id> and <!channel>. The text is turned into React
// elements, never into HTML, so markup in a message shows as text.
const pattern = new RegExp(
  [
    "`([^`\\n]+)`", // 1 code
    "\\*\\*(.+?)\\*\\*", // 2 bold
    "(?<![\\w*])\\*([^*\\s](?:[^*\\n]*?[^*\\s])?)\\*(?![\\w*])", // 3 italic
    "(?<!\\w)_([^_\\s](?:[^_\\n]*?[^_\\s])?)_(?!\\w)", // 4 italic
    "\\[([^\\]\\n]+)\\]\\((https?:\\/\\/[^\\s)]+)\\)", // 5 link text, 6 link url
    "(https?:\\/\\/[^\\s<>]+)", // 7 bare url
    "<@([^>\\s]+)>", // 8 mention
    "<!channel>", // channel mention
  ].join("|"),
  "g",
);

const trailingPunctuation = /[.,;:!?)\]]+$/;

interface Context {
  nameOf(userId: string): string;
  me: string;
}

function link(href: string, label: ReactNode) {
  return (
    <a href={href} target="_blank" rel="noopener noreferrer nofollow">
      {label}
    </a>
  );
}

function render(text: string, ctx: Context): ReactNode[] {
  const out: ReactNode[] = [];
  let last = 0;
  for (const m of text.matchAll(pattern)) {
    const at = m.index;
    if (at > last) out.push(text.slice(last, at));
    const key = out.length;
    if (m[1] !== undefined) {
      out.push(<code key={key}>{m[1]}</code>);
    } else if (m[2] !== undefined) {
      out.push(<strong key={key}>{render(m[2], ctx)}</strong>);
    } else if (m[3] !== undefined || m[4] !== undefined) {
      out.push(<em key={key}>{render((m[3] ?? m[4])!, ctx)}</em>);
    } else if (m[5] !== undefined) {
      out.push(<Fragment key={key}>{link(m[6]!, m[5])}</Fragment>);
    } else if (m[7] !== undefined) {
      const url = m[7].replace(trailingPunctuation, "");
      out.push(<Fragment key={key}>{link(url, url)}</Fragment>);
      if (url.length < m[7].length) out.push(m[7].slice(url.length));
    } else if (m[8] !== undefined) {
      out.push(
        <span key={key} className={m[8] === ctx.me ? "mention me" : "mention"}>
          @{ctx.nameOf(m[8])}
        </span>,
      );
    } else {
      // `<!channel>` notifies everyone, me included.
      out.push(
        <span key={key} className="mention me">
          @channel
        </span>,
      );
    }
    last = at + m[0].length;
  }
  if (last < text.length) out.push(text.slice(last));
  return out;
}

export function Markdown({ text, nameOf, me }: { text: string; nameOf: (userId: string) => string; me: string }) {
  return <>{render(text, { nameOf, me })}</>;
}
