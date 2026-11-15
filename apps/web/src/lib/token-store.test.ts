import { describe, expect, it } from "vitest";
import { createLocalStorageTokenStore } from "./token-store";

function fakeStorage(initial?: string) {
  const map = new Map<string, string>(initial === undefined ? [] : [["samnak.session", initial]]);
  return {
    getItem: (k: string) => map.get(k) ?? null,
    setItem: (k: string, v: string) => void map.set(k, v),
    removeItem: (k: string) => void map.delete(k),
  };
}

const data = { accessToken: "a", refreshToken: "r", deviceId: "d", workspaceId: "w" };

describe("localStorage token store", () => {
  it("round-trips and clears", () => {
    const storage = fakeStorage();
    const store = createLocalStorageTokenStore(storage);
    expect(store.load()).toBeNull();
    store.save(data);
    expect(store.load()).toEqual(data);
    store.save(null);
    expect(store.load()).toBeNull();
  });

  it("treats corrupt or incomplete data as signed out", () => {
    expect(createLocalStorageTokenStore(fakeStorage("{nope")).load()).toBeNull();
    expect(createLocalStorageTokenStore(fakeStorage(JSON.stringify({ accessToken: "a" }))).load()).toBeNull();
  });
});
