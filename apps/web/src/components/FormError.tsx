export function FormError({ message }: { message: string | null | undefined }) {
  // The live region stays mounted so screen readers announce text that appears later.
  return (
    <p className="m-0 mb-4 text-sm text-destructive empty:hidden [[data-page]_&]:mb-0" role="alert">
      {message}
    </p>
  );
}
