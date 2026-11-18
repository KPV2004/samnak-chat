import { ApiError } from "@samnak/core";
import { describe, expect, it, vi } from "vitest";
import { queryClient, retryUnlessClientError } from "./main";

// main.tsx mounts the app when it loads; the tests only need its QueryClient and retry policy.
vi.mock("react-dom/client", () => ({ createRoot: () => ({ render: () => undefined }) }));

const api = (status: number) => new ApiError(status, "x", "server text");

describe("retryUnlessClientError", () => {
  it.each([400, 401, 403, 404, 409, 422])("does not retry a %i answer", (status) => {
    expect(retryUnlessClientError(0, api(status))).toBe(false);
  });

  it.each([500, 502, 503])("retries a %i up to the default three times", (status) => {
    expect(retryUnlessClientError(0, api(status))).toBe(true);
    expect(retryUnlessClientError(2, api(status))).toBe(true);
    expect(retryUnlessClientError(3, api(status))).toBe(false);
  });

  it("retries a network failure up to the default three times", () => {
    const network = new TypeError("Failed to fetch");
    expect(retryUnlessClientError(0, network)).toBe(true);
    expect(retryUnlessClientError(3, network)).toBe(false);
  });
});

describe("queryClient", () => {
  it("uses the policy for every query", () => {
    expect(queryClient.getDefaultOptions().queries?.retry).toBe(retryUnlessClientError);
  });
});
