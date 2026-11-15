import type { Meta } from "./meta";

// Minimal structural fetch types: this package must not depend on DOM typings.
export interface FetchInit {
  method?: string;
  headers?: Record<string, string>;
  body?: string;
}

export interface FetchResponse {
  ok: boolean;
  status: number;
  statusText: string;
  json(): Promise<unknown>;
}

export type FetchLike = (url: string, init?: FetchInit) => Promise<FetchResponse>;

export interface ApiClientOptions {
  baseUrl: string;
  getToken?: () => string | null | undefined;
  /** Sent as `X-Workspace-ID` when it returns a value. */
  getWorkspaceId?: () => string | null | undefined;
  /**
   * Called on a 401 for an authenticated request, with the token that was rejected.
   * Resolve `true` to retry the request once with the current token.
   */
  onUnauthorized?: (rejectedToken: string) => Promise<boolean>;
  fetch?: FetchLike;
}

export interface ApiRequestInit extends FetchInit {
  /** Skip the `Authorization` header and the refresh-and-retry on 401 (login, register, refresh). */
  anonymous?: boolean;
}

export interface ApiErrorBody {
  error: { code: string; message: string };
}

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export interface ApiClient {
  request<T>(path: string, init?: ApiRequestInit): Promise<T>;
  getMeta(): Promise<Meta>;
}

export function createApiClient(opts: ApiClientOptions): ApiClient {
  const baseUrl = opts.baseUrl.replace(/\/+$/, "");

  async function send(path: string, init: ApiRequestInit): Promise<{ res: FetchResponse; token: string | null }> {
    const doFetch = opts.fetch ?? (globalThis as unknown as { fetch: FetchLike }).fetch;
    const { anonymous, ...fetchInit } = init;
    const headers: Record<string, string> = { ...fetchInit.headers, Accept: "application/json" };
    const token = anonymous ? null : (opts.getToken?.() ?? null);
    if (token) headers.Authorization = `Bearer ${token}`;
    const workspaceId = opts.getWorkspaceId?.();
    if (workspaceId && !anonymous) headers["X-Workspace-ID"] = workspaceId;
    return { res: await doFetch(`${baseUrl}${path}`, { ...fetchInit, headers }), token };
  }

  async function request<T>(path: string, init: ApiRequestInit = {}): Promise<T> {
    const first = await send(path, init);
    let res = first.res;
    if (res.status === 401 && first.token && opts.onUnauthorized && (await opts.onUnauthorized(first.token))) {
      res = (await send(path, init)).res;
    }
    if (!res.ok) {
      let code = "internal";
      let message = res.statusText || `HTTP ${res.status}`;
      try {
        const body = (await res.json()) as Partial<ApiErrorBody>;
        if (body.error) {
          code = body.error.code;
          message = body.error.message;
        }
      } catch {
        // body was not JSON; keep defaults
      }
      throw new ApiError(res.status, code, message);
    }
    if (res.status === 204) return undefined as T;
    return (await res.json()) as T;
  }

  return {
    request,
    getMeta: () => request<Meta>("/api/v1/meta"),
  };
}

export function getMeta(client: ApiClient): Promise<Meta> {
  return client.getMeta();
}
