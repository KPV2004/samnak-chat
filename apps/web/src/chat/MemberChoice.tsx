import type { Member } from "@samnak/core";
import { useId } from "react";
import { useChat } from "../state/chat";
import { useWorkspaceMembers } from "./names";

interface Props {
  legend: string;
  hint?: string;
  selected: string[];
  onChange(ids: string[]): void;
  /** Shown when nobody else is in the workspace. */
  empty: string;
}

/** Checkboxes of the workspace members other than me. */
export function MemberChoice({ legend, hint, selected, onChange, empty }: Props) {
  const me = useChat()?.userId;
  const { data } = useWorkspaceMembers();
  const hintId = useId();
  const others: Member[] = (data ?? []).filter((m) => m.user_id !== me && m.status === "active");

  function toggle(id: string, on: boolean) {
    onChange(on ? [...selected, id] : selected.filter((x) => x !== id));
  }

  return (
    <fieldset className="member-choice" aria-describedby={hint ? hintId : undefined}>
      <legend>{legend}</legend>
      {hint && (
        <small id={hintId} className="hint">
          {hint}
        </small>
      )}
      {data && others.length === 0 && <p className="muted">{empty}</p>}
      <ul>
        {others.map((m) => (
          <li key={m.user_id}>
            <label>
              <input
                type="checkbox"
                checked={selected.includes(m.user_id)}
                onChange={(e) => toggle(m.user_id, e.target.checked)}
              />{" "}
              {m.display_name || m.email}
            </label>
          </li>
        ))}
      </ul>
    </fieldset>
  );
}
