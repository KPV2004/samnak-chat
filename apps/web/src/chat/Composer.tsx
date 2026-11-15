import { useId, useRef, useState } from "react";
import { useT } from "../i18n/useT";
import { useChat } from "../state/chat";

export const maxLength = 4000;

interface Props {
  channelId: string;
  /** Sends replies under this top-level message instead of into the channel. */
  threadRootId?: string;
  label?: string;
}

export function Composer({ channelId, threadRootId, label }: Props) {
  const t = useT();
  const store = useChat()?.store;
  const [text, setText] = useState("");
  const boxRef = useRef<HTMLTextAreaElement>(null);
  const counterId = useId();
  const hintId = useId();

  const length = [...text].length;
  const tooLong = length > maxLength;
  const canSend = text.trim() !== "" && !tooLong && store !== undefined;

  function submit() {
    if (!canSend) return;
    store.send(channelId, text, threadRootId ? { thread_root_id: threadRootId } : {});
    setText("");
    boxRef.current?.focus();
  }

  function onKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    // Enter while an IME (Thai, Japanese) is composing confirms the word, it does not send.
    if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault();
      submit();
    }
  }

  return (
    <form
      className="chat-composer"
      onSubmit={(e) => {
        e.preventDefault();
        submit();
      }}
    >
      <label className="visually-hidden" htmlFor={`${counterId}-box`}>
        {label ?? t("chat.composer.label")}
      </label>
      <textarea
        id={`${counterId}-box`}
        ref={boxRef}
        rows={2}
        value={text}
        onChange={(e) => setText(e.target.value)}
        onKeyDown={onKeyDown}
        aria-describedby={`${hintId} ${counterId}`}
        aria-invalid={tooLong || undefined}
      />
      <div className="chat-composer-bar">
        <small id={hintId} className="muted">
          {t("chat.composer.hint")}
        </small>
        <small id={counterId} className={tooLong ? "form-error" : "muted"}>
          {t("chat.counter", { n: length.toLocaleString("en-US") })}
        </small>
        <button type="submit" disabled={!canSend}>
          {t("chat.send")}
        </button>
      </div>
      <p className="form-error" role="alert">
        {tooLong ? t("chat.tooLong") : null}
      </p>
    </form>
  );
}
