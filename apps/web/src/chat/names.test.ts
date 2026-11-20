import type { Member } from "@samnak/core";
import { describe, expect, it } from "vitest";
import { pictureOf } from "./names";

function member(userId: string, avatarUrl: string): Member {
  return {
    user_id: userId,
    display_name: userId,
    email: `${userId}@example.com`,
    avatar_url: avatarUrl,
    role: "member",
    status: "active",
    joined_at: "2026-01-01T00:00:00Z",
  };
}

const me = { id: "me", avatar_url: "https://cdn.example.com/me.png" };

describe("pictureOf", () => {
  it("shows another member's picture from the member list", () => {
    const members = [member("bob", "https://cdn.example.com/bob.png"), member("ann", "")];
    expect(pictureOf("bob", members, me)).toBe("https://cdn.example.com/bob.png");
  });

  it("is empty for a member without a picture, or one the list does not have", () => {
    const members = [member("ann", "")];
    expect(pictureOf("ann", members, me)).toBe("");
    expect(pictureOf("nobody", members, me)).toBe("");
  });

  it("uses the signed-in user's own profile even when the member list is older", () => {
    const members = [member("me", "https://cdn.example.com/old.png")];
    expect(pictureOf("me", members, me)).toBe("https://cdn.example.com/me.png");
  });

  it("still shows the signed-in user's picture while the member list loads", () => {
    expect(pictureOf("me", undefined, me)).toBe("https://cdn.example.com/me.png");
    expect(pictureOf("bob", undefined, me)).toBe("");
  });
});
