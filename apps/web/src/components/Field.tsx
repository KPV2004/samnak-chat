import { useId, type InputHTMLAttributes, type ReactNode } from "react";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

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
    // The bottom margin spaces fields in the old pages; a <Page> lays its children out with `gap` instead.
    <div className="mb-4 grid gap-1.5 [[data-page]_&]:mb-0">
      <Label htmlFor={id}>{label}</Label>
      {children ? (
        children({ id, describedBy, invalid: Boolean(error) })
      ) : (
        <Input id={id} aria-invalid={error ? true : undefined} aria-describedby={describedBy} {...input} />
      )}
      {hint && (
        <small id={hintId} className="text-xs text-muted-foreground">
          {hint}
        </small>
      )}
      <small id={errorId} className="text-xs text-destructive empty:hidden" aria-live="polite">
        {error}
      </small>
    </div>
  );
}
