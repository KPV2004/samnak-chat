export function FormError({ message }: { message: string | null | undefined }) {
  // The live region stays mounted so screen readers announce text that appears later.
  return (
    <p className="form-error" role="alert">
      {message}
    </p>
  );
}
