import { describe, expect, it, vi } from "vitest";
import { createApiClient } from "./client";
import { createM1Api } from "./m1";

function setup() {
  const fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 200, statusText: "", json: async () => ({}) });
  const api = createM1Api(createApiClient({ baseUrl: "", getToken: () => "t", fetch: fetchMock }));
  const last = () => {
    const [url, init] = fetchMock.mock.calls.at(-1)!;
    return { url: url as string, method: init.method as string | undefined, body: init.body ? JSON.parse(init.body) : undefined };
  };
  return { api, last };
}

describe("M1 api calls", () => {
  it("maps each call to its method, path and body", async () => {
    const { api, last } = setup();
    await api.register({ email: "a@b.co", password: "12345678", display_name: "A" });
    expect(last()).toMatchObject({ url: "/api/v1/auth/register", method: "POST", body: { display_name: "A" } });
    await api.updateMe({ locale: "en" });
    expect(last()).toMatchObject({ url: "/api/v1/auth/me", method: "PATCH", body: { locale: "en" } });
    await api.revokeDevice("d 1");
    expect(last()).toMatchObject({ url: "/api/v1/auth/devices/d%201", method: "DELETE" });
    await api.createWorkspace({ name: "Acme", slug: "acme" });
    expect(last()).toMatchObject({ url: "/api/v1/workspace", method: "POST" });
    await api.listMembers({ cursor: "c1", limit: 20 });
    expect(last().url).toBe("/api/v1/workspace/members?cursor=c1&limit=20");
    await api.listMembers();
    expect(last().url).toBe("/api/v1/workspace/members");
    await api.updateMember("u1", { role: "admin" });
    expect(last()).toMatchObject({ url: "/api/v1/workspace/members/u1", method: "PATCH", body: { role: "admin" } });
    await api.removeMember("u1");
    expect(last()).toMatchObject({ url: "/api/v1/workspace/members/u1", method: "DELETE" });
    await api.createInvite({ role: "member" });
    expect(last()).toMatchObject({ url: "/api/v1/workspace/invites", method: "POST", body: { role: "member" } });
    await api.revokeInvite("i1");
    expect(last()).toMatchObject({ url: "/api/v1/workspace/invites/i1", method: "DELETE" });
    await api.acceptInvite("tok");
    expect(last()).toMatchObject({ url: "/api/v1/workspace/invites/accept", method: "POST", body: { token: "tok" } });
  });
});
