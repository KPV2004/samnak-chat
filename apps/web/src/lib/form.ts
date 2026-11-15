/** Reads every named field of an uncontrolled form as a trimmed-free string map. */
export function readForm(form: HTMLFormElement): Record<string, string> {
  const out: Record<string, string> = {};
  new FormData(form).forEach((value, key) => {
    if (typeof value === "string") out[key] = value;
  });
  return out;
}

export function focusField(form: HTMLFormElement, name: string) {
  const el = form.elements.namedItem(name);
  if (el instanceof HTMLElement) el.focus();
}

export const emailPattern = /^\S+@\S+\.\S+$/;
