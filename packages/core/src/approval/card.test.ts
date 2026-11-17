import { describe, expect, it } from "vitest";
import { newestCard, parseApprovalCard, type ApprovalCardPayload } from "./card";

const card: ApprovalCardPayload = {
  request_id: "r1",
  type: "leave",
  status: "pending",
  summary: "Annual leave 2026-02-02 to 2026-02-03",
  fields: { leave_type: "annual", reason: "trip" },
  requester_id: "u1",
  requester_name: "Mali",
  approver_id: "u2",
  approver_name: "Somchai",
  reason: "",
  decided_at: null,
  version: 100,
};

describe("approval card payload", () => {
  it("reads a payload the server sends", () => {
    expect(parseApprovalCard(card)).toEqual(card);
  });

  it("rejects what is not an approval card", () => {
    expect(parseApprovalCard(null)).toBeNull();
    expect(parseApprovalCard("x")).toBeNull();
    expect(parseApprovalCard({ ...card, request_id: undefined })).toBeNull();
    expect(parseApprovalCard({ ...card, status: "weird" })).toBeNull();
  });

  it("tolerates missing optional text and fields", () => {
    const parsed = parseApprovalCard({ request_id: "r1", type: "general", status: "approved", version: 5 });
    expect(parsed).toMatchObject({ request_id: "r1", summary: "", fields: {}, requester_name: "", reason: "", decided_at: null });
  });

  it("keeps the newest payload by version", () => {
    const old = { ...card, version: 1 };
    const fresh = { ...card, status: "approved" as const, version: 2 };
    expect(newestCard(old, fresh)).toBe(fresh);
    expect(newestCard(fresh, old)).toBe(fresh);
    expect(newestCard(null, old)).toBe(old);
    expect(newestCard(fresh, null)).toBe(fresh);
    const same = { ...fresh };
    expect(newestCard(fresh, same)).toBe(same);
  });
});
