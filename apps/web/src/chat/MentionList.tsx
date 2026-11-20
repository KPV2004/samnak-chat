import { Megaphone } from "lucide-react";
import { initialOf } from "../components/UserAvatar";
import type { MentionOption } from "./mentions";

interface Props {
  id: string;
  label: string;
  options: MentionOption[];
  active: number;
  onPick(option: MentionOption): void;
}

/** What leads an option: a command has none, `@channel` a megaphone, a member their avatar. */
function Lead({ option }: { option: MentionOption }) {
  if (option.token.startsWith("/")) return null;
  if (option.token === "<!channel>") {
    return (
      <span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground" aria-hidden="true">
        <Megaphone className="size-3.5" />
      </span>
    );
  }
  // The initial is drawn by CSS so the option's text stays just the name.
  return (
    <span
      data-initial={initialOf(option.label)}
      aria-hidden="true"
      className="flex size-6 shrink-0 items-center justify-center rounded-full bg-primary/10 text-xs font-medium text-primary before:content-[attr(data-initial)]"
    />
  );
}

/** The suggestions above the composer: members after @, commands after /. */
export function MentionList({ id, label, options, active, onPick }: Props) {
  return (
    <ul
      id={id}
      role="listbox"
      aria-label={label}
      className="absolute inset-x-0 bottom-full z-10 mb-2 max-h-60 overflow-y-auto rounded-lg border bg-popover p-1 text-sm text-popover-foreground shadow-md"
    >
      {options.map((o, i) => (
        <li
          key={o.token}
          id={`${id}-${i}`}
          role="option"
          aria-selected={i === active}
          // Keep the focus in the text box so the caret and the keyboard stay where they were.
          onMouseDown={(e) => e.preventDefault()}
          onClick={() => onPick(o)}
          className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 transition-colors hover:bg-accent/60 active:bg-accent aria-selected:bg-accent aria-selected:text-accent-foreground"
        >
          <Lead option={o} />
          <span className={o.token.startsWith("/") ? "font-mono text-[0.85em] font-medium" : "font-medium"}>{o.label}</span>
          {o.detail && <small className="min-w-0 text-xs text-muted-foreground">{o.detail}</small>}
        </li>
      ))}
    </ul>
  );
}
