import { useId, type InputHTMLAttributes, type ReactNode } from "react";

interface FieldProps extends Omit<InputHTMLAttributes<HTMLInputElement>, "id" | "children"> {
  label: string;
  error?: string;
  hint?: string;
  /** Replaces the `<input>`, e.g. with a `<select>`; it receives the ids to wire up. */
  children?: (ids: { id: string; describedBy: string | undefined; invalid: boolean }) => ReactNode;
}

/** A labelled control. The error is announced when it appears and tied to the control. */
export function Field({ label, error, hint, children, ...input }: FieldProps) {
  const id = useId();
  const errorId = `${id}-error`;
  const hintId = `${id}-hint`;
  const describedBy = [error ? errorId : null, hint ? hintId : null].filter(Boolean).join(" ") || undefined;
  return (
    <div className="field">
      <label htmlFor={id}>{label}</label>
      {children ? (
        children({ id, describedBy, invalid: Boolean(error) })
      ) : (
        <input id={id} aria-invalid={error ? true : undefined} aria-describedby={describedBy} {...input} />
      )}
      {hint && (
        <small id={hintId} className="hint">
          {hint}
        </small>
      )}
      <small id={errorId} className="error" aria-live="polite">
        {error}
      </small>
    </div>
  );
}
