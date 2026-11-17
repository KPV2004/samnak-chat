import { describe, expect, it } from "vitest";
import { ApiError, createApiClient, type FetchInit, type FetchResponse } from "./client";
import { createExtensionsApi } from "./extensions";
import { createTaskApi, type Task } from "./task";

const task: Task = {
  id: "t1",
  channel_id: "c1",
  message_id: "m1",
  source_message_id: null,
  title: "Prepare the report",
  description: "",
  creator_id: "u1",
  assignee_id: "u2",
  due_at: null,
  priority: "normal",
  status: "todo",
  created_at: "2026-01-01T09:00:00Z",
  updated_at: "2026-01-01T09:00:00Z",
};

const ok = (body: unknown, status = 200): FetchResponse => ({
  ok: true,
  status,
  statusText: "",
  json: async () => JSON.parse(JSON.stringify(body)),
});

const noMatch = (): FetchResponse => ({
  ok: false,
  status: 403,
  statusText: "forbidden",
  json: async () => ({ error: { code: "forbidden", message: "only the creator" } }),
});

function setup() {
  const requests: { line: string; body: unknown }[] = [];
  const fetch = async (url: string, init: FetchInit = {}): Promise<FetchResponse> => {
    requests.push({ line: `${init.method ?? "GET"} ${url}`, body: init.body ? JSON.parse(init.body) : undefined });
    const path = url.split("?")[0]!;
    if (path === "/api/v1/task/items" && init.method === "POST") return ok(task, 201);
    if (path === "/api/v1/task/items") return ok({ items: [task], next_cursor: "abc" });
    if (path === "/api/v1/task/items/denied") return noMatch();
    if (path.startsWith("/api/v1/task/items/")) return ok(task);
    if (path === "/api/v1/chat/extensions") {
      return ok({
        commands: [{ name: "task", usage: "/task <title> [@assignee]", description: "Create a task", module: "task" }],
        message_actions: [{ id: "task.create_from_message", module: "task" }],
      });
    }
    if (path === "/api/v1/chat/channels/c1/commands") return ok({ text: "" });
    return noMatch();
  };
  const client = createApiClient({ baseUrl: "", fetch });
  return { tasks: createTaskApi(client), extensions: createExtensionsApi(client), requests };
}

describe("task api", () => {
  it("lists with the filters the plan defines, skipping the ones not set", async () => {
    const { tasks, requests } = setup();
    await expect(tasks.listTasks()).resolves.toMatchObject({ items: [task], next_cursor: "abc" });
    await tasks.listTasks({ assignee: "me", status: ["todo", "doing"], due: "week", channel_id: "c/1", cursor: "x", limit: 20 });
    expect(requests.map((r) => r.line)).toEqual([
      "GET /api/v1/task/items",
      "GET /api/v1/task/items?assignee=me&status=todo%2Cdoing&channel_id=c%2F1&due=week&cursor=x&limit=20",
    ]);
  });

  it("creates a task with the body as given", async () => {
    const { tasks, requests } = setup();
    await expect(tasks.createTask({ channel_id: "c1", title: "Prepare the report", assignee_id: "u2" })).resolves.toEqual(task);
    expect(requests[0]).toEqual({
      line: "POST /api/v1/task/items",
      body: { channel_id: "c1", title: "Prepare the report", assignee_id: "u2" },
    });
  });

  it("gets one task and encodes the id", async () => {
    const { tasks, requests } = setup();
    await tasks.getTask("t/1");
    expect(requests[0]!.line).toBe("GET /api/v1/task/items/t%2F1");
  });

  it("patches only the fields given, and null clears the assignee", async () => {
    const { tasks, requests } = setup();
    await tasks.updateTask("t1", { status: "done", assignee_id: null });
    expect(requests[0]).toEqual({ line: "PATCH /api/v1/task/items/t1", body: { status: "done", assignee_id: null } });
  });

  it("surfaces the server's error body", async () => {
    const { tasks } = setup();
    const error = await tasks.updateTask("denied", { status: "done" }).catch((e: unknown) => e);
    expect(error).toBeInstanceOf(ApiError);
    expect(error).toMatchObject({ status: 403, code: "forbidden", message: "only the creator" });
  });
});

describe("chat extensions api", () => {
  it("lists the commands and message actions", async () => {
    const { extensions, requests } = setup();
    const found = await extensions.listExtensions();
    expect(found.commands[0]).toMatchObject({ name: "task", module: "task" });
    expect(found.message_actions).toEqual([{ id: "task.create_from_message", module: "task" }]);
    expect(requests[0]!.line).toBe("GET /api/v1/chat/extensions");
  });

  it("runs a command in a channel, with the thread when there is one", async () => {
    const { extensions, requests } = setup();
    await expect(extensions.runCommand("c1", { name: "task", args: "do it", thread_root_id: "m9" })).resolves.toEqual({ text: "" });
    expect(requests[0]).toEqual({
      line: "POST /api/v1/chat/channels/c1/commands",
      body: { name: "task", args: "do it", thread_root_id: "m9" },
    });
  });
});
