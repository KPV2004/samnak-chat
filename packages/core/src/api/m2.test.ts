import { describe, expect, it, vi } from "vitest";
import { createApiClient } from "./client";
import { createM2Api } from "./m2";

function setup(status = 200, body: unknown = {}) {
  const fetchMock = vi.fn().mockResolvedValue({ ok: status < 400, status, statusText: "", json: async () => body });
  const api = createM2Api(createApiClient({ baseUrl: "http://x", fetch: fetchMock }));
  const last = () => {
    const [url, init] = fetchMock.mock.calls.at(-1)!;
    return { url: url as string, method: init.method as string, body: init.body ? JSON.parse(init.body) : undefined };
  };
  return { api, last };
}

describe("m2 api", () => {
  it("lists joined channels by default and public ones on request", async () => {
    const { api, last } = setup(200, { items: [] });
    await api.listChannels();
    expect(last()).toMatchObject({ url: "http://x/api/v1/chat/channels", method: "GET" });
    await api.listChannels("public");
    expect(last().url).toBe("http://x/api/v1/chat/channels?scope=public");
  });

  it("sends after_seq=0 for a catch-up from the start", async () => {
    const { api, last } = setup(200, { items: [], has_more: false });
    await api.listMessages("c1", { after_seq: 0, limit: 100 });
    expect(last().url).toBe("http://x/api/v1/chat/channels/c1/messages?after_seq=0&limit=100");
  });

  it("omits undefined query values and encodes ids", async () => {
    const { api, last } = setup(200, { items: [], has_more: false });
    await api.listMessages("a/b", { before_seq: 9, limit: undefined });
    expect(last().url).toBe("http://x/api/v1/chat/channels/a%2Fb/messages?before_seq=9");
  });

  it("posts a message with its client_msg_id", async () => {
    const { api, last } = setup(201, { id: "m1" });
    await api.sendMessage("c1", { client_msg_id: "k1", body: "hi" });
    expect(last()).toEqual({
      url: "http://x/api/v1/chat/channels/c1/messages",
      method: "POST",
      body: { client_msg_id: "k1", body: "hi" },
    });
  });

  it("pins and unpins a channel with PUT and DELETE", async () => {
    const { api, last } = setup(204);
    await api.pinChannel("c1");
    expect(last()).toMatchObject({ url: "http://x/api/v1/chat/channels/c1/pin", method: "PUT" });
    await api.unpinChannel("c1");
    expect(last()).toMatchObject({ url: "http://x/api/v1/chat/channels/c1/pin", method: "DELETE" });
  });

  it("posts the read position", async () => {
    const { api, last } = setup(204);
    await api.markRead("c1", 7);
    expect(last()).toMatchObject({ url: "http://x/api/v1/chat/channels/c1/read", method: "POST", body: { seq: 7 } });
  });

  it("covers edit, delete, join and leave", async () => {
    const { api, last } = setup(200, {});
    await api.editMessage("m1", "new");
    expect(last()).toMatchObject({ url: "http://x/api/v1/chat/messages/m1", method: "PATCH", body: { body: "new" } });
    await api.deleteMessage("m1");
    expect(last()).toMatchObject({ url: "http://x/api/v1/chat/messages/m1", method: "DELETE" });
    await api.joinChannel("c1");
    expect(last()).toMatchObject({ url: "http://x/api/v1/chat/channels/c1/join", method: "POST" });
    await api.leaveChannel("c1");
    expect(last()).toMatchObject({ url: "http://x/api/v1/chat/channels/c1/leave", method: "POST" });
  });

  it("adds and removes a reaction, encoding the emoji", async () => {
    const { api, last } = setup(204);
    await api.addReaction("m1", "👍");
    expect(last()).toMatchObject({ url: "http://x/api/v1/chat/messages/m1/reactions", method: "POST", body: { emoji: "👍" } });
    await api.removeReaction("m1", "👍");
    expect(last()).toMatchObject({
      url: `http://x/api/v1/chat/messages/m1/reactions/${encodeURIComponent("👍")}`,
      method: "DELETE",
    });
  });

  it("opens a direct message and creates a channel", async () => {
    const { api, last } = setup(201, {});
    await api.openDm(["u2"]);
    expect(last()).toMatchObject({ url: "http://x/api/v1/chat/dms", method: "POST", body: { user_ids: ["u2"] } });
    await api.createChannel({ type: "private", name: "ops", member_ids: ["u2"] });
    expect(last()).toMatchObject({
      url: "http://x/api/v1/chat/channels",
      method: "POST",
      body: { type: "private", name: "ops", member_ids: ["u2"] },
    });
  });

  it("lists channel members and fetches one channel", async () => {
    const { api, last } = setup(200, { items: [] });
    await api.listChannelMembers("c1");
    expect(last()).toMatchObject({ url: "http://x/api/v1/chat/channels/c1/members", method: "GET" });
    await api.getChannel("c1");
    expect(last()).toMatchObject({ url: "http://x/api/v1/chat/channels/c1", method: "GET" });
  });

  it("asks for the replies of a thread", async () => {
    const { api, last } = setup(200, { items: [], has_more: false });
    await api.listMessages("c1", { thread_root_id: "r1", limit: 100 });
    expect(last().url).toBe("http://x/api/v1/chat/channels/c1/messages?thread_root_id=r1&limit=100");
  });
});
