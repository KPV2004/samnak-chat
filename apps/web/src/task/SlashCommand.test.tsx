import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { channel, chatRoutes, errorRes, message, renderApp, type Routes } from "../test-utils";

const general = channel("c1", { name: "general", last_seq: 1 });

const extensions = {
  commands: [
    { name: "task", usage: "/task <title> [@assignee]", description: "Create a task", module: "task" },
    { name: "ticket", usage: "/ticket <subject>", description: "Open a ticket", module: "ticket" },
  ],
  message_actions: [{ id: "task.create_from_message", module: "task" }],
};

function routes(modules: string[] = ["chat", "task"], extra: Routes = {}): Routes {
  return {
    ...chatRoutes,
    "GET /api/v1/meta": () => ({ name: "samnak-chat", version: "x", modules }),
    "GET /api/v1/chat/extensions": () => extensions,
    "GET /api/v1/chat/channels": () => ({ items: [general] }),
    "GET /api/v1/chat/channels/c1": () => general,
    "GET /api/v1/chat/channels/c1/members": () => ({ items: [] }),
    "GET /api/v1/chat/channels/c1/messages": () => ({ items: [message("c1", 1)], has_more: false }),
    "POST /api/v1/chat/channels/c1/read": () => undefined,
    "POST /api/v1/chat/channels/c1/messages": (body) =>
      new Response(JSON.stringify(message("c1", 2, { sender_id: "u1", body: body.body })), { status: 201 }),
    ...extra,
  };
}

const box = () => screen.findByRole("textbox", { name: "Message" });

describe("slash commands", () => {
  it("lists the commands of the modules /meta lists, and no others", async () => {
    const user = userEvent.setup();
    renderApp({ route: "/chat/c1", routes: routes() });
    await user.type(await box(), "/");
    const list = await screen.findByRole("listbox", { name: "Commands" });
    expect(within(list).getAllByRole("option")).toHaveLength(1);
    expect(within(list).getByRole("option", { name: /\/task <title>/ })).toHaveTextContent("Create a task");
    expect(screen.queryByText(/ticket/)).not.toBeInTheDocument();
  });

  it("offers no commands when /meta lacks task", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/chat/c1", routes: routes(["chat"]) });
    await user.type(await box(), "/ta");
    await waitFor(() => expect(app.called("GET /api/v1/chat/extensions")).toHaveLength(1));
    await act(async () => {});
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });

  it("inserts a picked command and sends it to the commands endpoint with its arguments", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/chat/c1",
      routes: routes(undefined, {
        "POST /api/v1/chat/channels/c1/commands": () => ({ text: "" }),
      }),
    });
    await user.type(await box(), "/ta");
    const list = await screen.findByRole("listbox", { name: "Commands" });
    await user.click(within(list).getByRole("option", { name: /\/task/ }));
    expect(await box()).toHaveValue("/task ");
    await user.type(await box(), "Prepare the report{Enter}");
    await waitFor(() => expect(app.called("POST /api/v1/chat/channels/c1/commands")).toHaveLength(1));
    expect(app.called("POST /api/v1/chat/channels/c1/commands")[0]!.body).toEqual({
      name: "task",
      args: "Prepare the report",
    });
    expect(app.called("POST /api/v1/chat/channels/c1/messages")).toHaveLength(0);
    await waitFor(async () => expect(await box()).toHaveValue(""));
  });

  it("sends a complete command on Enter, and shows what the command says back", async () => {
    const user = userEvent.setup();
    renderApp({
      route: "/chat/c1",
      routes: routes(undefined, {
        "POST /api/v1/chat/channels/c1/commands": () => ({ text: "Usage: /task <title> [@assignee]" }),
      }),
    });
    await user.type(await box(), "/task{Enter}");
    expect(await screen.findByText("Usage: /task <title> [@assignee]")).toBeInTheDocument();
  });

  it("keeps the text and says why when the command is refused", async () => {
    const user = userEvent.setup();
    renderApp({
      route: "/chat/c1",
      routes: routes(undefined, {
        "POST /api/v1/chat/channels/c1/commands": () => errorRes(400, "invalid"),
      }),
    });
    await user.type(await box(), "/task{Enter}");
    expect(await screen.findByText("Some details are not valid. Check them and try again.")).toBeInTheDocument();
    expect(await box()).toHaveValue("/task");
  });

  it("sends a slash word that is not a registered command as plain text", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/chat/c1", routes: routes() });
    await user.type(await box(), "/nope hello{Enter}");
    await waitFor(() => expect(app.called("POST /api/v1/chat/channels/c1/messages")).toHaveLength(1));
    expect(app.called("POST /api/v1/chat/channels/c1/messages")[0]!.body.body).toBe("/nope hello");
    expect(app.called("POST /api/v1/chat/channels/c1/commands")).toHaveLength(0);
  });
});
