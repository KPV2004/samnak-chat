import { describe, expect, it, vi } from "vitest";
import { ApiError, createApiClient, getMeta } from "./client";

function jsonResponse(body: unknown, status = 200) {
  return { ok: status < 400, status, statusText: "", json: async () => body };
}

describe("getMeta", () => {
  it("fetches /api/v1/meta and returns the typed body", async () => {
    const meta = { name: "samnak-chat", version: "0.0.0-dev", modules: ["auth", "chat"] };
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(meta));
    const client = createApiClient({ baseUrl: "http://x/", getToken: () => null, fetch: fetchMock });

    await expect(getMeta(client)).resolves.toEqual(meta);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0]![0]).toBe("http://x/api/v1/meta");
  });

  it("sends the bearer token when present", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ name: "n", version: "v", modules: [] }));
    const client = createApiClient({ baseUrl: "", getToken: () => "tok", fetch: fetchMock });
    await client.getMeta();
    expect(fetchMock.mock.calls[0]![1].headers.Authorization).toBe("Bearer tok");
  });

  it("throws ApiError with the server error code", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(jsonResponse({ error: { code: "internal", message: "boom" } }, 500));
    const client = createApiClient({ baseUrl: "", getToken: () => null, fetch: fetchMock });
    await expect(client.getMeta()).rejects.toMatchObject({ status: 500, code: "internal", message: "boom" });
    await expect(client.getMeta()).rejects.toBeInstanceOf(ApiError);
  });
});
