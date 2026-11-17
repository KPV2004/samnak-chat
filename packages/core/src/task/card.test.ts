import { describe, expect, it } from "vitest";
import { parseTaskCard } from "./card";

const payload = {
  task_id: "t1",
  title: "Prepare the report",
  status: "doing",
  priority: "high",
  assignee_id: "u2",
  assignee_name: "Somchai",
  due_at: "2026-02-01T00:00:00Z",
  version: 1767225600000,
};

describe("parseTaskCard", () => {
  it("reads a task.item payload", () => {
    expect(parseTaskCard(payload)).toEqual(payload);
  });

  it("allows no assignee and no due date", () => {
    expect(parseTaskCard({ ...payload, assignee_id: null, assignee_name: "", due_at: null })).toMatchObject({
      assignee_id: null,
      due_at: null,
    });
    expect(parseTaskCard({ ...payload, assignee_id: undefined, due_at: undefined })).toMatchObject({
      assignee_id: null,
      due_at: null,
    });
  });

  it("returns null for anything that is not a task payload", () => {
    expect(parseTaskCard(null)).toBeNull();
    expect(parseTaskCard("x")).toBeNull();
    expect(parseTaskCard({})).toBeNull();
    expect(parseTaskCard({ ...payload, status: "archived" })).toBeNull();
    expect(parseTaskCard({ ...payload, task_id: 4 })).toBeNull();
  });
});
