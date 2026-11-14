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
  fetch?: FetchLike;
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
  request<T>(path: string, init?: FetchInit): Promise<T>;
  getMeta(): Promise<Meta>;
}

export function createApiClient(opts: ApiClientOptions): ApiClient {
  const baseUrl = opts.baseUrl.replace(/\/+$/, "");

  async function request<T>(path: string, init: FetchInit = {}): Promise<T> {
    const doFetch = opts.fetch ?? (globalThis as unknown as { fetch: FetchLike }).fetch;
    const headers: Record<string, string> = { ...init.headers, Accept: "application/json" };
    const token = opts.getToken?.();
    if (token) headers.Authorization = `Bearer ${token}`;

    const res = await doFetch(`${baseUrl}${path}`, { ...init, headers });
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
