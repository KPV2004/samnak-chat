import { describe, expect, it } from "vitest";
import { ApiError, createApiClient, type FetchInit, type FetchResponse } from "./client";
import { createApprovalApi, type ApprovalRequest, type Group } from "./m4-approval";

const request: ApprovalRequest = {
  id: "r1",
  type: "leave",
  status: "pending",
  requester_id: "u1",
  approver_id: "u2",
  payload: { leave_type: "annual" },
  reason: "",
  decided_at: null,
  message_id: null,
  created_at: "2026-01-01T09:00:00Z",
  updated_at: "2026-01-01T09:00:00Z",
};
const group: Group = { id: "g1", name: "Sales", member_count: 2, manager_ids: ["u2"] };

const reply = (status: number, body: unknown): FetchResponse => ({
  ok: status < 400,
  status,
  statusText: "",
  json: async () => JSON.parse(JSON.stringify(body ?? null)),
});

function setup() {
  const requests: { line: string; body: unknown; headers: Record<string, string> | undefined }[] = [];
  const fetch = async (url: string, init: FetchInit = {}): Promise<FetchResponse> => {
    requests.push({ line: `${init.method ?? "GET"} ${url}`, body: init.body ? JSON.parse(init.body) : undefined, headers: init.headers });
    if (url.includes("/missing")) return reply(404, { error: { code: "not_found", message: "no such request" } });
    if (url.includes("/groups") && (init.method === "PUT" || init.method === "DELETE")) return reply(204, null);
    if (url.includes("/groups")) return reply(200, url.includes("/members") ? { items: [] } : { items: [group] });
    return reply(200, url.includes("/requests?") || url.endsWith("/requests") ? { items: [request], next_cursor: "" } : request);
  };
  return { api: createApprovalApi(createApiClient({ baseUrl: "", fetch })), requests };
}

const lines = (r: { line: string }[]) => r.map((x) => x.line);

describe("approval api", () => {
  it("lists requests with the role, status and paging query", async () => {
    const { api, requests } = setup();
    await expect(api.listRequests()).resolves.toMatchObject({ items: [request] });
    await api.listRequests({ role: "approver", status: "pending", cursor: "c1", limit: 10 });
    expect(lines(requests)).toEqual([
      "GET /api/v1/approval/requests",
      "GET /api/v1/approval/requests?role=approver&status=pending&cursor=c1&limit=10",
    ]);
  });

  it("creates a request with a JSON body", async () => {
    const { api, requests } = setup();
    const input = { type: "general" as const, payload: { subject: "Laptop", details: "" }, approver_id: "u2", attachment_ids: ["a1"] };
    await api.createRequest(input);
    expect(requests[0]).toMatchObject({ line: "POST /api/v1/approval/requests", body: input });
    expect(requests[0]!.headers).toMatchObject({ "Content-Type": "application/json" });
  });

  it("gets, approves and cancels by id, encoding it", async () => {
    const { api, requests } = setup();
    await api.getRequest("r/1");
    await api.approve("r1");
    await api.cancel("r1");
    expect(lines(requests)).toEqual([
      "GET /api/v1/approval/requests/r%2F1",
      "POST /api/v1/approval/requests/r1/approve",
      "POST /api/v1/approval/requests/r1/cancel",
    ]);
  });

  it("rejects with a reason", async () => {
    const { api, requests } = setup();
    await api.reject("r1", "no budget");
    expect(requests[0]).toMatchObject({ line: "POST /api/v1/approval/requests/r1/reject", body: { reason: "no budget" } });
  });

  it("presigns an upload and gets an attachment URL", async () => {
    const { api, requests } = setup();
    await api.requestUpload({ file_name: "a.pdf", size: 3, mime_type: "application/pdf" });
    await api.getAttachmentUrl("a1");
    expect(lines(requests)).toEqual(["POST /api/v1/approval/uploads", "GET /api/v1/approval/attachments/a1"]);
    expect(requests[0]!.body).toEqual({ file_name: "a.pdf", size: 3, mime_type: "application/pdf" });
  });

  it("calls the group endpoints", async () => {
    const { api, requests } = setup();
    await expect(api.listGroups()).resolves.toEqual({ items: [group] });
    await api.createGroup("Sales");
    await api.renameGroup("g1", "Ops");
    await api.deleteGroup("g1");
    await api.listGroupMembers("g1");
    await expect(api.setGroupMembers("g1", [{ user_id: "u2", is_manager: true }])).resolves.toBeUndefined();
    expect(requests.map((r) => [r.line, r.body])).toEqual([
      ["GET /api/v1/workspace/groups", undefined],
      ["POST /api/v1/workspace/groups", { name: "Sales" }],
      ["PATCH /api/v1/workspace/groups/g1", { name: "Ops" }],
      ["DELETE /api/v1/workspace/groups/g1", undefined],
      ["GET /api/v1/workspace/groups/g1/members", undefined],
      ["PUT /api/v1/workspace/groups/g1/members", { members: [{ user_id: "u2", is_manager: true }] }],
    ]);
  });

  it("surfaces the server's error body", async () => {
    const { api } = setup();
    const error = await api.getRequest("missing").catch((e: unknown) => e);
    expect(error).toBeInstanceOf(ApiError);
    expect(error).toMatchObject({ status: 404, code: "not_found" });
  });
});
