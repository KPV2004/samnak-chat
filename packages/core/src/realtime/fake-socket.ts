import type { SocketLike } from "./client";

/** Test double for a WebSocket: records what the client sends and lets a test play the server. */
export class FakeSocket implements SocketLike {
  static instances: FakeSocket[] = [];
  static reset() {
    FakeSocket.instances = [];
  }
  static last(): FakeSocket {
    const s = FakeSocket.instances.at(-1);
    if (!s) throw new Error("no socket was created");
    return s;
  }

  onopen: (() => void) | null = null;
  onmessage: ((ev: { data: unknown }) => void) | null = null;
  onclose: ((ev: { code: number }) => void) | null = null;
  onerror: ((ev: unknown) => void) | null = null;
  sent: { type: string; data?: unknown }[] = [];
  closed: { code?: number } | null = null;

  constructor(readonly url: string) {
    FakeSocket.instances.push(this);
  }

  send(data: string) {
    this.sent.push(JSON.parse(data));
  }

  close(code?: number) {
    this.closed = { code };
    // Like a real socket, closing ends in a close event.
    this.onclose?.({ code: code ?? 1005 });
  }

  // Server side
  open() {
    this.onopen?.();
  }
  receive(type: string, data?: unknown) {
    this.onmessage?.({ data: JSON.stringify({ type, data }) });
  }
  receiveRaw(data: unknown) {
    this.onmessage?.({ data });
  }
  drop(code = 1006) {
    this.onclose?.({ code });
  }
}
