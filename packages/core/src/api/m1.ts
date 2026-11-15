// M1 types come from the server's OpenAPI document (npm run gen:api); this
// file re-exports them under stable names and holds the typed calls. Each call
// is typed by the generated operation for its path and method, so renaming or
// reshaping an endpoint on the server makes `npm run build` fail here.
import type { ApiClient } from "./client";
import type { components, paths } from "./schema";

type Schemas = components["schemas"];

export type User = Schemas["UserBody"];
export type TokenPair = Schemas["TokenPairBody"];
export type Device = Schemas["DeviceBody"];
export type Workspace = Schemas["Workspace"];
export type Member = Schemas["Member"];
export type Invite = Schemas["Invite"];

export type Locale = NonNullable<Schemas["UpdateMeInputBody"]["locale"]>;
export type Platform = Schemas["LoginInputBody"]["platform"];
export type Role = Workspace["role"];
export type InviteRole = Schemas["CreateInviteInputBody"]["role"];
export type MemberStatus = Member["status"];

export type RegisterInput = Schemas["RegisterInputBody"];
export type LoginInput = Schemas["LoginInputBody"];
export type ProfilePatch = Schemas["UpdateMeInputBody"];
export type CreateWorkspaceInput = Schemas["CreateInputBody"];
export type MemberPatch = Schemas["UpdateMemberInputBody"];
export type CreateInviteInput = Schemas["CreateInviteInputBody"];

// Not `Schemas["ListMembersOutputBody"]`: Huma numbers equal names across modules, so that one is chat's.
export type MemberPage = Exclude<paths["/workspace/members"]["get"], undefined>["responses"][200]["content"]["application/json"];
export type ListMembersParams = NonNullable<paths["/workspace/members"]["get"]["parameters"]["query"]>;

type Method = "get" | "post" | "patch" | "delete";
type Operation<P extends keyof paths, M extends Method> = Exclude<paths[P][M], undefined>;
/** The methods a path really has: the generator marks the others `?: never`. */
type MethodsOf<P extends keyof paths> = { [M in Method]: [Operation<P, M>] extends [never] ? never : M }[Method];

type JsonBody<O> = O extends { requestBody: { content: { "application/json": infer B } } } ? B : undefined;
type JsonResponse<O> = O extends { responses: { 200: { content: { "application/json": infer R } } } }
  ? R
  : O extends { responses: { 201: { content: { "application/json": infer R } } } }
    ? R
    : void;

interface CallOptions<B> {
  body?: B;
  params?: Record<string, string>;
  query?: Record<string, string | number | undefined>;
  anonymous?: boolean;
}

const base = "/api/v1";
const enc = encodeURIComponent;

/** Calls an operation by the key it has in the generated `paths`. */
function call<P extends keyof paths, M extends MethodsOf<P>>(
  client: ApiClient,
  method: M,
  path: P,
  opts: CallOptions<JsonBody<Operation<P, M>>> = {},
): Promise<JsonResponse<Operation<P, M>>> {
  let url = base + (path as string).replace(/\{(\w+)\}/g, (_, name: string) => enc(opts.params?.[name] ?? ""));
  const qs = Object.entries(opts.query ?? {})
    .filter(([, v]) => v !== undefined && v !== "" && v !== 0)
    .map(([k, v]) => `${k}=${enc(String(v))}`)
    .join("&");
  if (qs) url += `?${qs}`;
  const init: Parameters<ApiClient["request"]>[1] = { method: method.toUpperCase(), anonymous: opts.anonymous };
  if (opts.body !== undefined) {
    init.headers = { "Content-Type": "application/json" };
    init.body = JSON.stringify(opts.body);
  }
  return client.request(url, init);
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
  listMembers(params?: ListMembersParams): Promise<MemberPage>;
  updateMember(userId: string, patch: MemberPatch): Promise<Member>;
  removeMember(userId: string): Promise<void>;
  createInvite(input: CreateInviteInput): Promise<Invite>;
  listInvites(): Promise<{ items: Invite[] }>;
  revokeInvite(id: string): Promise<void>;
  acceptInvite(token: string): Promise<Workspace>;
}

export function createM1Api(client: ApiClient): M1Api {
  return {
    register: (input) => call(client, "post", "/auth/register", { body: input, anonymous: true }),
    login: (input) => call(client, "post", "/auth/login", { body: input, anonymous: true }),
    refresh: (refreshToken) =>
      call(client, "post", "/auth/refresh", { body: { refresh_token: refreshToken }, anonymous: true }),
    logout: (allDevices) => call(client, "post", "/auth/logout", { body: { all_devices: allDevices } }),
    getMe: () => call(client, "get", "/auth/me"),
    updateMe: (patch) => call(client, "patch", "/auth/me", { body: patch }),
    listDevices: () => call(client, "get", "/auth/devices"),
    revokeDevice: (id) => call(client, "delete", "/auth/devices/{id}", { params: { id } }),
    createWorkspace: (input) => call(client, "post", "/workspace", { body: input }),
    listWorkspaces: () => call(client, "get", "/workspace"),
    listMembers: (params = {}) => call(client, "get", "/workspace/members", { query: params }),
    updateMember: (userId, patch) => call(client, "patch", "/workspace/members/{userId}", { params: { userId }, body: patch }),
    removeMember: (userId) => call(client, "delete", "/workspace/members/{userId}", { params: { userId } }),
    createInvite: (input) => call(client, "post", "/workspace/invites", { body: input }),
    listInvites: () => call(client, "get", "/workspace/invites"),
    revokeInvite: (id) => call(client, "delete", "/workspace/invites/{id}", { params: { id } }),
    acceptInvite: (token) => call(client, "post", "/workspace/invites/accept", { body: { token } }),
  };
}
