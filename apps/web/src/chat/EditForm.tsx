import type { Message } from "@samnak/core";
import { useId, useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { useT } from "../i18n/useT";
import { useChat } from "../state/chat";
import { maxLength } from "./Composer";
import { toReadable, toTokens } from "./mentions";
import { useNameOf } from "./names";

interface Props {
  message: Message;
  onDone(): void;
}

export function EditForm({ message, onDone }: Props) {
  const t = useT();
  const chat = useChat();
  const nameOf = useNameOf();
  const id = useId();
  // Mentions show as names; the picks turn them back into tokens on save.
  const [start] = useState(() => toReadable(message.body, nameOf));
  const [text, setText] = useState(start.text);
  const [saving, setSaving] = useState(false);
  const [failed, setFailed] = useState(false);

  const body = toTokens(text, start.picks);
  const length = [...body].length;
  const valid = text.trim() !== "" && length <= maxLength;

  async function save() {
    if (!chat || !valid || saving) return;
    setSaving(true);
    setFailed(false);
    try {
      chat.store.applyMessage(await chat.api.editMessage(message.id, body));
      toast.success(t("chat.toast.edited"));
      onDone();
    } catch {
      setFailed(true);
      setSaving(false);
    }
  }

  return (
    <form
      className="mt-1 grid gap-2"
      onSubmit={(e) => {
        e.preventDefault();
        void save();
      }}
    >
      <label className="sr-only" htmlFor={id}>
        {t("chat.editLabel")}
      </label>
      <Textarea
        id={id}
        autoFocus
        rows={2}
        value={text}
        aria-invalid={length > maxLength || undefined}
        onChange={(e) => setText(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Escape") onDone();
          else if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
            e.preventDefault();
            void save();
          }
        }}
      />
      <div className="flex items-center justify-end gap-2">
        <Button type="button" variant="outline" size="sm" onClick={onDone}>
          {t("cancel")}
        </Button>
        <Button type="submit" size="sm" disabled={!valid || saving}>
          {t("save")}
        </Button>
      </div>
      {(failed || length > maxLength) && (
        <p className="text-xs text-destructive" role="alert">
          {failed ? t("chat.actionError") : t("chat.tooLong")}
        </p>
      )}
    </form>
  );
}
