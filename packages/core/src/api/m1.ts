// Hand-written M1 types and calls, from the API table in tasks/plan.md.
// They move to the generated OpenAPI types at checkpoint B.
import type { ApiClient } from "./client";

export type Locale = "th" | "en";
export type Platform = "web" | "desktop" | "ios" | "android";
export type Role = "owner" | "admin" | "member" | "guest";
export type InviteRole = Exclude<Role, "owner">;
export type MemberStatus = "active" | "deactivated";

export interface User {
  id: string;
  email: string;
  display_name: string;
  avatar_url: string;
  locale: Locale;
  timezone: string;
  created_at: string;
}

export interface TokenPair {
  access_token: string;
  refresh_token: string;
  expires_in: number;
  device_id: string;
  user: User;
}

export interface Device {
  id: string;
  name: string;
  platform: Platform;
  last_seen_at: string;
  current: boolean;
}

export interface Workspace {
  id: string;
  name: string;
  slug: string;
  role: Role;
}

export interface Member {
  user_id: string;
  display_name: string;
  email: string;
  role: Role;
  status: MemberStatus;
  joined_at: string;
}

export interface Invite {
  id: string;
  email?: string;
  role: Role;
  expires_at: string;
  created_at: string;
  /** Present only in the response to create. */
  token?: string;
}

export interface RegisterInput {
  email: string;
  password: string;
  display_name: string;
}

export interface LoginInput {
  email: string;
  password: string;
  device_name: string;
  platform: Platform;
}

export interface ProfilePatch {
  display_name?: string;
  avatar_url?: string;
  locale?: Locale;
  timezone?: string;
}

export interface CreateWorkspaceInput {
  name: string;
  slug: string;
}

export interface MemberPatch {
  role?: Role;
  status?: MemberStatus;
}

export interface CreateInviteInput {
  email?: string;
  role: InviteRole;
}

export interface Page<T> {
  items: T[];
  next_cursor?: string | null;
}

const base = "/api/v1";
const enc = encodeURIComponent;

function json(method: string, body: unknown) {
  return { method, headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) };
}

export interface M1Api {
  register(input: RegisterInput): Promise<User>;
  login(input: LoginInput): Promise<TokenPair>;
  refresh(refreshToken: string): Promise<TokenPair>;
  logout(allDevices: boolean): Promise<void>;
  getMe(): Promise<User>;
  updateMe(patch: ProfilePatch): Promise<User>;
  listDevices(): Promise<{ items: Device[] }>;
  revokeDevice(id: string): Promise<void>;
  createWorkspace(input: CreateWorkspaceInput): Promise<Workspace>;
  listWorkspaces(): Promise<{ items: Workspace[] }>;
  listMembers(params?: { cursor?: string; limit?: number }): Promise<Page<Member>>;
  updateMember(userId: string, patch: MemberPatch): Promise<Member>;
  removeMember(userId: string): Promise<void>;
  createInvite(input: CreateInviteInput): Promise<Invite>;
  listInvites(): Promise<{ items: Invite[] }>;
  revokeInvite(id: string): Promise<void>;
  acceptInvite(token: string): Promise<Workspace>;
}

export function createM1Api(client: ApiClient): M1Api {
  return {
    register: (input) => client.request(`${base}/auth/register`, { ...json("POST", input), anonymous: true }),
    login: (input) => client.request(`${base}/auth/login`, { ...json("POST", input), anonymous: true }),
    refresh: (refreshToken) =>
      client.request(`${base}/auth/refresh`, { ...json("POST", { refresh_token: refreshToken }), anonymous: true }),
    logout: (allDevices) => client.request(`${base}/auth/logout`, json("POST", { all_devices: allDevices })),
    getMe: () => client.request(`${base}/auth/me`),
    updateMe: (patch) => client.request(`${base}/auth/me`, json("PATCH", patch)),
    listDevices: () => client.request(`${base}/auth/devices`),
    revokeDevice: (id) => client.request(`${base}/auth/devices/${enc(id)}`, { method: "DELETE" }),
    createWorkspace: (input) => client.request(`${base}/workspace`, json("POST", input)),
    listWorkspaces: () => client.request(`${base}/workspace`),
    listMembers: (params = {}) => {
      const q: string[] = [];
      if (params.cursor) q.push(`cursor=${enc(params.cursor)}`);
      if (params.limit) q.push(`limit=${params.limit}`);
      const qs = q.join("&");
      return client.request(`${base}/workspace/members${qs ? `?${qs}` : ""}`);
    },
    updateMember: (userId, patch) => client.request(`${base}/workspace/members/${enc(userId)}`, json("PATCH", patch)),
    removeMember: (userId) => client.request(`${base}/workspace/members/${enc(userId)}`, { method: "DELETE" }),
    createInvite: (input) => client.request(`${base}/workspace/invites`, json("POST", input)),
    listInvites: () => client.request(`${base}/workspace/invites`),
    revokeInvite: (id) => client.request(`${base}/workspace/invites/${enc(id)}`, { method: "DELETE" }),
    acceptInvite: (token) => client.request(`${base}/workspace/invites/accept`, json("POST", { token })),
  };
}
