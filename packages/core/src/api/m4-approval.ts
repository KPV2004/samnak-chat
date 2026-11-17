// M4 (approval and groups) types come from the server's OpenAPI document
// (npm run gen:api); this file re-exports them under stable names and holds the
// calls, the way m3.ts does. The task and chat-extension calls of M4 live with
// their own web task.
import type { ApiClient } from "./client";
import type { components } from "./schema";

type Schemas = components["schemas"];

export type ApprovalRequest = Schemas["ApprovalRequest"];
export type ApprovalRequestDetail = Schemas["ApprovalRequestDetail"];
export type ApprovalHistoryEntry = Schemas["ApprovalHistoryEntry"];
export type ApprovalAttachment = Schemas["ApprovalAttachment"];
export type ApprovalPage = Schemas["ApprovalListOutputBody"];
export type ApprovalType = ApprovalRequest["type"];
export type ApprovalStatus = ApprovalRequest["status"];
export type ApprovalHistoryAction = ApprovalHistoryEntry["action"];
export type CreateApprovalInput = Schemas["ApprovalCreateInputBody"];
export type ApprovalUploadInput = Schemas["ApprovalUploadInputBody"];
export type ApprovalUpload = Schemas["ApprovalUploadOutputBody"];
export type ApprovalAttachmentUrl = Schemas["ApprovalAttachmentURLOutputBody"];

export type Group = Schemas["Group"];
export type GroupMember = Schemas["GroupMember"];
/** One entry of the replacement set sent to `PUT /workspace/groups/{id}/members`. */
export type GroupMemberInput = Schemas["Item"];

export interface ListApprovalsParams {
  /** requester: my requests; approver: waiting for me; both when omitted. */
  role?: "requester" | "approver";
  /** Comma-separated: pending, approved, rejected, cancelled. */
  status?: string;
  /** `next_cursor` of the previous page. */
  cursor?: string;
  limit?: number;
}

export interface ApprovalApi {
  listRequests(params?: ListApprovalsParams): Promise<ApprovalPage>;
  createRequest(input: CreateApprovalInput): Promise<ApprovalRequest>;
  getRequest(id: string): Promise<ApprovalRequestDetail>;
  approve(id: string): Promise<ApprovalRequest>;
  reject(id: string, reason: string): Promise<ApprovalRequest>;
  cancel(id: string): Promise<ApprovalRequest>;
  requestUpload(input: ApprovalUploadInput): Promise<ApprovalUpload>;
  getAttachmentUrl(id: string): Promise<ApprovalAttachmentUrl>;

  listGroups(): Promise<{ items: Group[] }>;
  createGroup(name: string): Promise<Group>;
  renameGroup(id: string, name: string): Promise<Group>;
  deleteGroup(id: string): Promise<void>;
  listGroupMembers(id: string): Promise<{ items: GroupMember[] }>;
  /** Replaces the whole member set. */
  setGroupMembers(id: string, members: GroupMemberInput[]): Promise<void>;
}

const base = "/api/v1";
const enc = encodeURIComponent;

function json(method: string, body: unknown) {
  return { method, headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) };
}

export function createApprovalApi(client: ApiClient): ApprovalApi {
  const action = (id: string, verb: string, body?: unknown) =>
    client.request<ApprovalRequest>(`${base}/approval/requests/${enc(id)}/${verb}`, body === undefined ? { method: "POST" } : json("POST", body));

  return {
    listRequests(params = {}) {
      const qs = Object.entries({ role: params.role, status: params.status, cursor: params.cursor, limit: params.limit })
        .filter(([, value]) => value !== undefined)
        .map(([key, value]) => `${key}=${enc(String(value))}`)
        .join("&");
      return client.request(`${base}/approval/requests${qs ? `?${qs}` : ""}`);
    },
    createRequest: (input) => client.request(`${base}/approval/requests`, json("POST", input)),
    getRequest: (id) => client.request(`${base}/approval/requests/${enc(id)}`),
    approve: (id) => action(id, "approve"),
    reject: (id, reason) => action(id, "reject", { reason }),
    cancel: (id) => action(id, "cancel"),
    requestUpload: (input) => client.request(`${base}/approval/uploads`, json("POST", input)),
    getAttachmentUrl: (id) => client.request(`${base}/approval/attachments/${enc(id)}`),

    listGroups: () => client.request(`${base}/workspace/groups`),
    createGroup: (name) => client.request(`${base}/workspace/groups`, json("POST", { name })),
    renameGroup: (id, name) => client.request(`${base}/workspace/groups/${enc(id)}`, json("PATCH", { name })),
    deleteGroup: (id) => client.request(`${base}/workspace/groups/${enc(id)}`, { method: "DELETE" }),
    listGroupMembers: (id) => client.request(`${base}/workspace/groups/${enc(id)}/members`),
    setGroupMembers: (id, members) => client.request(`${base}/workspace/groups/${enc(id)}/members`, json("PUT", { members })),
  };
}
