import { describe, expect, it } from "vitest";
import type { Channel } from "../api/m2";
import { newChannelState } from "./channel";
import { sortConversations } from "./order";

function state(id: string, patch: Partial<Channel> = {}) {
  return newChannelState({
    id,
    type: "public",
    name: id,
    topic: "",
    last_seq: 0,
    last_read_seq: 0,
    unread_count: 0,
    mention_count: 0,
    member_count: 2,
    dm_user_ids: [],
    archived_at: null,
    pinned_at: null,
    last_message: null,
    created_at: "2026-01-01T00:00:00Z",
    ...patch,
  });
}

const msg = (at: string) => ({ id: "m", sender_id: "u", kind: "text", preview: "", created_at: at });
const ids = (list: { channel: { id: string } }[]) => list.map((s) => s.channel.id);

describe("sortConversations", () => {
  it("puts pinned first (newest pin first), then the newest message, then the name", () => {
    const list = sortConversations([
      state("b-old", { last_message: msg("2026-02-01T00:00:00Z") }),
      state("empty-b"),
      state("pin-old", { pinned_at: "2026-03-01T00:00:00Z", last_message: msg("2026-05-01T00:00:00Z") }),
      state("new", { last_message: msg("2026-04-01T00:00:00Z") }),
      state("pin-new", { pinned_at: "2026-03-02T00:00:00Z" }),
      state("empty-a"),
    ]);
    expect(ids(list)).toEqual(["pin-new", "pin-old", "new", "b-old", "empty-a", "empty-b"]);
  });

  it("breaks a tie by the title the caller gives, then by id, and does not change its input", () => {
    const input = [state("2", { type: "dm", name: "" }), state("1", { type: "dm", name: "" }), state("3", { type: "dm", name: "" })];
    const titles: Record<string, string> = { "1": "Zed", "2": "Ann", "3": "Ann" };
    const list = sortConversations(input, (s) => titles[s.channel.id]!);
    expect(ids(list)).toEqual(["2", "3", "1"]);
    expect(ids(input)).toEqual(["2", "1", "3"]);
  });
});
