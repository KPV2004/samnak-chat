import type { MentionOption } from "./mentions";

interface Props {
  id: string;
  label: string;
  options: MentionOption[];
  active: number;
  onPick(option: MentionOption): void;
}

export function MentionList({ id, label, options, active, onPick }: Props) {
  return (
    <ul id={id} role="listbox" aria-label={label} className="chat-mentions">
      {options.map((o, i) => (
        <li
          key={o.token}
          id={`${id}-${i}`}
          role="option"
          aria-selected={i === active}
          // Keep the focus in the text box so the caret and the keyboard stay where they were.
          onMouseDown={(e) => e.preventDefault()}
          onClick={() => onPick(o)}
        >
          {o.label}
          {o.detail && <small className="muted"> {o.detail}</small>}
        </li>
      ))}
    </ul>
  );
}
