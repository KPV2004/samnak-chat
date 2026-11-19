import { describe, expect, expectTypeOf, it, vi } from "vitest";
import { createApiClient } from "./client";
import type { AuditEntry, Device, Invite, Member, MemberPage, TokenPair, User, Workspace } from "./m1";
import { createM1Api } from "./m1";
import type { components } from "./schema";

type Schemas = components["schemas"];

// `npm run build` type-checks this file, so these fail the build when the
// server's OpenAPI document and the types the web app uses drift apart.
describe("M1 types come from the generated schema", () => {
  it("are the generated types", () => {
    expectTypeOf<User>().toEqualTypeOf<Schemas["UserBody"]>();
    expectTypeOf<TokenPair>().toEqualTypeOf<Schemas["TokenPairBody"]>();
    expectTypeOf<Device>().toEqualTypeOf<Schemas["DeviceBody"]>();
    expectTypeOf<Workspace>().toEqualTypeOf<Schemas["Workspace"]>();
    expectTypeOf<Member>().toEqualTypeOf<Schemas["Member"]>();
    expectTypeOf<Invite>().toEqualTypeOf<Schemas["Invite"]>();
  });

  it("type the calls by the generated operations", () => {
    const api = createM1Api(createApiClient({ baseUrl: "" }));
    expectTypeOf(api.register).returns.resolves.toEqualTypeOf<Schemas["UserBody"]>();
    expectTypeOf(api.login).parameter(0).toEqualTypeOf<Schemas["LoginInputBody"]>();
    expectTypeOf(api.listMembers).returns.resolves.toEqualTypeOf<MemberPage>();
    expectTypeOf(api.listAudit).returns.resolves.toEqualTypeOf<Schemas["ListAuditOutputBody"]>();
    expectTypeOf<AuditEntry>().toEqualTypeOf<Schemas["AuditEntry"]>();
    // Chat's channel members page is also called ListMembersOutputBody in the document; this must be the workspace's.
    expectTypeOf<MemberPage["items"][number]>().toEqualTypeOf<Schemas["Member"]>();
    expectTypeOf(api.acceptInvite).returns.resolves.toEqualTypeOf<Schemas["Workspace"]>();
    expectTypeOf(api.createInvite).parameter(0).toEqualTypeOf<Schemas["CreateInviteInputBody"]>();
    expectTypeOf(api.resetPassword).parameter(0).toEqualTypeOf<Schemas["ResetPasswordInputBody"]>();
    expectTypeOf(api.forgotPassword).parameter(0).toEqualTypeOf<string>();
    expectTypeOf(api.verifyEmail).parameter(0).toEqualTypeOf<string>();
    expectTypeOf(api.resendVerification).returns.resolves.toEqualTypeOf<void>();
  });
});

function setup() {
  const fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 200, statusText: "", json: async () => ({}) });
  const api = createM1Api(createApiClient({ baseUrl: "", getToken: () => "t", fetch: fetchMock }));
  const last = () => {
    const [url, init] = fetchMock.mock.calls.at(-1)!;
    return {
      url: url as string,
      method: init.method as string | undefined,
      body: init.body ? JSON.parse(init.body) : undefined,
      headers: init.headers as Record<string, string>,
    };
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
    await api.listAudit({ cursor: "a/1+2", limit: 25 });
    expect(last()).toMatchObject({ url: "/api/v1/workspace/audit?cursor=a%2F1%2B2&limit=25", method: "GET" });
    await api.listAudit({ cursor: "", limit: 50 });
    expect(last().url).toBe("/api/v1/workspace/audit?limit=50");
    await api.listAudit();
    expect(last().url).toBe("/api/v1/workspace/audit");
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

  it("maps the email and password calls to their paths", async () => {
    const { api, last } = setup();
    await api.verifyEmail("tok");
    expect(last()).toMatchObject({ url: "/api/v1/auth/email/verify", method: "POST", body: { token: "tok" } });
    await api.resendVerification();
    expect(last()).toMatchObject({ url: "/api/v1/auth/email/verify/resend", method: "POST" });
    await api.forgotPassword("a@b.co");
    expect(last()).toMatchObject({ url: "/api/v1/auth/password/forgot", method: "POST", body: { email: "a@b.co" } });
    await api.resetPassword({ token: "tok", password: "12345678" });
    expect(last()).toMatchObject({ url: "/api/v1/auth/password/reset", method: "POST", body: { token: "tok", password: "12345678" } });
  });

  it("sends no session token on the calls made before sign-in", async () => {
    const { api, last } = setup();
    await api.verifyEmail("tok");
    expect(last().headers).not.toHaveProperty("Authorization");
    await api.forgotPassword("a@b.co");
    expect(last().headers).not.toHaveProperty("Authorization");
    await api.resetPassword({ token: "tok", password: "12345678" });
    expect(last().headers).not.toHaveProperty("Authorization");
    await api.resendVerification();
    expect(last().headers).toHaveProperty("Authorization", "Bearer t");
  });
});
