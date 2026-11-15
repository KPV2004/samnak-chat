/** A readable name for this browser, shown in the device list. */
export function deviceName(userAgent: string | undefined = typeof navigator === "undefined" ? undefined : navigator.userAgent): string {
  const ua = userAgent ?? "";
  const browser = /Edg\//.test(ua)
    ? "Edge"
    : /Firefox\//.test(ua)
      ? "Firefox"
      : /Chrome\//.test(ua)
        ? "Chrome"
        : /Safari\//.test(ua)
          ? "Safari"
          : "Browser";
  return `${browser} (web)`;
}
