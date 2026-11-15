interface CryptoLike {
  getRandomValues(bytes: Uint8Array): Uint8Array;
}

/** A UUIDv7 (RFC 9562): 48-bit millisecond time, then random bits. */
export function uuidv7(now: number = Date.now()): string {
  const bytes = new Uint8Array(16);
  const crypto = (globalThis as unknown as { crypto?: CryptoLike }).crypto;
  if (crypto) crypto.getRandomValues(bytes);
  else for (let i = 0; i < 16; i++) bytes[i] = Math.floor(Math.random() * 256);
  for (let i = 5; i >= 0; i--) {
    bytes[i] = now % 256;
    now = Math.floor(now / 256);
  }
  bytes[6] = (bytes[6]! & 0x0f) | 0x70;
  bytes[8] = (bytes[8]! & 0x3f) | 0x80;
  const hex = Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}
