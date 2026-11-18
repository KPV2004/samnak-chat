import type { Member } from "@samnak/core";
import { useId } from "react";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
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
  const uid = useId();
  const hintId = useId();
  const others: Member[] = (data ?? []).filter((m) => m.user_id !== me && m.status === "active");

  function toggle(id: string, on: boolean) {
    onChange(on ? [...selected, id] : selected.filter((x) => x !== id));
  }

  return (
    <fieldset className="grid gap-2" aria-describedby={hint ? hintId : undefined}>
      <legend className="text-sm font-medium">{legend}</legend>
      {hint && (
        <small id={hintId} className="text-xs text-muted-foreground">
          {hint}
        </small>
      )}
      {data && others.length === 0 && <p className="text-sm text-muted-foreground">{empty}</p>}
      {others.length > 0 && (
        <ul className="max-h-72 divide-y overflow-y-auto rounded-lg border">
          {others.map((m) => {
            const id = `${uid}-${m.user_id}`;
            return (
              <li key={m.user_id}>
                <div className="flex items-center gap-3 px-3 py-2.5 hover:bg-muted/50">
                  <Checkbox
                    id={id}
                    checked={selected.includes(m.user_id)}
                    onCheckedChange={(on) => toggle(m.user_id, on === true)}
                  />
                  <Label htmlFor={id} className="cursor-pointer font-normal">
                    {m.display_name || m.email}
                  </Label>
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </fieldset>
  );
}
