import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { channel, chatRoutes, errorRes, message, mali, renderApp, somchai, type Routes } from "../test-utils";

const general = channel("c1", { name: "general", last_seq: 1 });
const request = message("c1", 1, { body: "Please prepare the report for Friday", sender_id: "u1" });

function routes(modules: string[] = ["chat", "task"], extra: Routes = {}): Routes {
  return {
    ...chatRoutes,
    "GET /api/v1/meta": () => ({ name: "samnak-chat", version: "x", modules }),
    "GET /api/v1/chat/extensions": () => ({
      commands: [],
      message_actions: [{ id: "task.create_from_message", module: "task" }],
    }),
    "GET /api/v1/chat/channels": () => ({ items: [general] }),
    "GET /api/v1/chat/channels/c1": () => general,
    "GET /api/v1/chat/channels/c1/members": () => ({ items: [mali, somchai] }),
    "GET /api/v1/chat/channels/c1/messages": () => ({ items: [request], has_more: false }),
    "POST /api/v1/chat/channels/c1/read": () => undefined,
    "POST /api/v1/task/items": (body) => ({ id: "t1", ...body, status: "todo" }),
    ...extra,
  };
}

async function openForm(user: ReturnType<typeof userEvent.setup>, extra: Routes = {}) {
  const app = renderApp({ route: "/chat/c1", routes: routes(undefined, extra) });
  const item = (await screen.findByText("Please prepare the report for Friday")).closest("li")!;
  await user.click(await within(item).findByRole("button", { name: "Create task" }));
  const form = screen.getByRole("form", { name: "New task from this message" });
  return { app, form };
}

describe("create task from a message", () => {
  it("opens with the message text as the title", async () => {
    const user = userEvent.setup();
    const { form } = await openForm(user);
    expect(within(form).getByRole("textbox", { name: "Title" })).toHaveValue("Please prepare the report for Friday");
  });

  it("creates the task from the message, with the assignee, priority and due date", async () => {
    const user = userEvent.setup();
    const { app, form } = await openForm(user);
    await user.clear(within(form).getByRole("textbox", { name: "Title" }));
    await user.type(within(form).getByRole("textbox", { name: "Title" }), "Report for Friday");
    await user.selectOptions(within(form).getByRole("combobox", { name: "Assignee" }), "u2");
    await user.selectOptions(within(form).getByRole("combobox", { name: "Priority" }), "high");
    await user.type(within(form).getByLabelText("Due date"), "2026-02-06");
    await user.click(within(form).getByRole("button", { name: "Create" }));

    await waitFor(() => expect(screen.queryByRole("form", { name: "New task from this message" })).not.toBeInTheDocument());
    const body = app.called("POST /api/v1/task/items")[0]!.body;
    expect(body).toMatchObject({
      channel_id: "c1",
      title: "Report for Friday",
      source_message_id: request.id,
      assignee_id: "u2",
      priority: "high",
    });
    // The end of the chosen day in the sender's time zone.
    expect(new Date(body.due_at as string).getTime()).toBe(new Date("2026-02-06T23:59:59").getTime());
  });

  it("sends no assignee, due date or priority unless they are chosen", async () => {
    const user = userEvent.setup();
    const { app, form } = await openForm(user);
    await user.click(within(form).getByRole("button", { name: "Create" }));
    await waitFor(() => expect(app.called("POST /api/v1/task/items")).toHaveLength(1));
    expect(app.called("POST /api/v1/task/items")[0]!.body).toEqual({
      channel_id: "c1",
      title: "Please prepare the report for Friday",
      source_message_id: request.id,
      priority: "normal",
    });
  });

  it("asks for a title and does not send an empty one", async () => {
    const user = userEvent.setup();
    const { app, form } = await openForm(user);
    await user.clear(within(form).getByRole("textbox", { name: "Title" }));
    await user.click(within(form).getByRole("button", { name: "Create" }));
    expect(await within(form).findByText("Enter a title")).toBeInTheDocument();
    expect(app.called("POST /api/v1/task/items")).toHaveLength(0);
  });

  it("stays open and says why when the server refuses", async () => {
    const user = userEvent.setup();
    const { form } = await openForm(user, { "POST /api/v1/task/items": () => errorRes(403, "forbidden") });
    await user.click(within(form).getByRole("button", { name: "Create" }));
    expect(await within(form).findByRole("alert")).toHaveTextContent("You are not allowed to do this.");
    expect(screen.getByRole("form", { name: "New task from this message" })).toBeInTheDocument();
  });

  it("closes without creating anything on Cancel", async () => {
    const user = userEvent.setup();
    const { app, form } = await openForm(user);
    await user.click(within(form).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("form", { name: "New task from this message" })).not.toBeInTheDocument();
    expect(app.called("POST /api/v1/task/items")).toHaveLength(0);
  });

  it("is not offered when /meta lacks task", async () => {
    const app = renderApp({ route: "/chat/c1", routes: routes(["chat"]) });
    const item = (await screen.findByText("Please prepare the report for Friday")).closest("li")!;
    await waitFor(() => expect(app.called("GET /api/v1/chat/extensions")).toHaveLength(1));
    await act(async () => {});
    expect(within(item).queryByRole("button", { name: "Create task" })).not.toBeInTheDocument();
  });
});
