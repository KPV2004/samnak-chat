import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { channel, chatRoutes, errorRes, message, renderApp, type Routes } from "../test-utils";

const general = channel("c1", { name: "general", last_seq: 1 });

const payload = {
  task_id: "t1",
  title: "Prepare the report",
  status: "doing",
  priority: "high",
  assignee_id: "u2",
  assignee_name: "Somchai",
  due_at: "2026-02-01T00:00:00Z",
  version: Date.parse("2026-01-01T09:00:00Z"),
};

const card = message("c1", 1, {
  kind: "task.item",
  body: "",
  fallback_text: "Task: Prepare the report",
  payload,
});

/** The task as the server returns it after a change, one minute later than the card. */
function task(patch: Record<string, unknown> = {}) {
  return {
    id: "t1",
    channel_id: "c1",
    message_id: card.id,
    source_message_id: null,
    title: "Prepare the report",
    description: "",
    creator_id: "u1",
    assignee_id: "u2",
    due_at: "2026-02-01T00:00:00Z",
    priority: "high",
    status: "doing",
    created_at: "2026-01-01T09:00:00Z",
    updated_at: "2026-01-01T09:01:00Z",
    ...patch,
  };
}

const modules = (names: string[]): Routes => ({
  "GET /api/v1/meta": () => ({ name: "samnak-chat", version: "x", modules: names }),
});

function routes(extra: Routes = {}): Routes {
  return {
    ...chatRoutes,
    ...modules(["chat", "task"]),
    "GET /api/v1/chat/channels": () => ({ items: [general] }),
    "GET /api/v1/chat/channels/c1": () => general,
    "GET /api/v1/chat/channels/c1/members": () => ({ items: [] }),
    "GET /api/v1/chat/channels/c1/messages": (_b, url) =>
      url.includes("thread_root_id") ? { items: [], has_more: false } : { items: [card], has_more: false },
    "POST /api/v1/chat/channels/c1/read": () => undefined,
    ...extra,
  };
}

/** Lets the `/meta` answer land, so a test that expects nothing to show does not pass too early. */
async function metaAnswered(app: { called: (key: string) => unknown[] }) {
  await waitFor(() => expect(app.called("GET /api/v1/meta").length).toBeGreaterThan(0));
  await act(async () => {});
}

describe("task card", () => {
  it("shows the task and its status, which can be changed", async () => {
    renderApp({ route: "/chat/c1", routes: routes() });
    const status = await screen.findByRole("combobox", { name: /Status/ });
    expect(status).toHaveValue("doing");
    const item = screen.getByText("Prepare the report").closest("li")!;
    expect(within(item).getByText("Somchai", { selector: "li" })).toBeInTheDocument();
    expect(within(item).getByText("High")).toBeInTheDocument();
  });

  it("sends a status change and shows the task as the server now has it", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/chat/c1",
      routes: routes({
        "PATCH /api/v1/task/items/t1": (body) => task({ status: body.status, updated_at: "2026-01-01T09:05:00Z" }),
      }),
    });
    const status = await screen.findByRole("combobox", { name: /Status/ });
    await user.selectOptions(status, "done");
    await waitFor(() => expect(screen.getByRole("combobox", { name: /Status/ })).toHaveValue("done"));
    expect(app.called("PATCH /api/v1/task/items/t1")[0]!.body).toEqual({ status: "done" });
  });

  it("says why a refused change failed and shows the task as the server has it", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/chat/c1",
      routes: routes({
        "PATCH /api/v1/task/items/t1": () => errorRes(403, "forbidden"),
        "GET /api/v1/task/items/t1": () => task({ status: "todo", updated_at: "2026-01-01T09:02:00Z" }),
      }),
    });
    const status = await screen.findByRole("combobox", { name: /Status/ });
    await user.selectOptions(status, "done");
    const item = screen.getByText("Prepare the report").closest("li")!;
    expect(await within(item).findByRole("alert")).toHaveTextContent("You are not allowed to do this.");
    await waitFor(() => expect(screen.getByRole("combobox", { name: /Status/ })).toHaveValue("todo"));
    expect(app.called("GET /api/v1/task/items/t1")).toHaveLength(1);
  });

  it("opens the card's thread from Comments", async () => {
    const user = userEvent.setup();
    renderApp({ route: "/chat/c1", routes: routes() });
    const item = (await screen.findByText("Prepare the report")).closest("li")!;
    await user.click(within(item).getByRole("button", { name: "Comments" }));
    expect(await screen.findByRole("complementary", { name: "Thread" })).toBeInTheDocument();
  });

  it("shows the fallback text, with no status to change, when /meta lacks task", async () => {
    const app = renderApp({ route: "/chat/c1", routes: routes(modules(["chat"])) });
    expect(await screen.findByText("Task: Prepare the report")).toBeInTheDocument();
    await metaAnswered(app);
    expect(screen.queryByRole("combobox", { name: /Status/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Comments" })).not.toBeInTheDocument();
  });

  it("shows the fallback text for a payload that is not a task", async () => {
    renderApp({
      route: "/chat/c1",
      routes: routes({
        "GET /api/v1/chat/channels/c1/messages": () => ({
          items: [message("c1", 1, { kind: "task.item", fallback_text: "Task: broken", payload: { title: 3 } })],
          has_more: false,
        }),
      }),
    });
    expect(await screen.findByText("Task: broken")).toBeInTheDocument();
    expect(screen.queryByRole("combobox", { name: /Status/ })).not.toBeInTheDocument();
  });
});
