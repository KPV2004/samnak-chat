import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { channel, chatRoutes, mali, message, renderApp, somchai, type Routes } from "../test-utils";

const general = channel("c1", { name: "general", last_seq: 1 });
const request = message("c1", 1, { body: "The printer on floor 3 is jammed", sender_id: "u2" });
const itQueue = {
  id: "q1",
  name: "IT",
  prefix: "IT",
  channel_id: null,
  agent_ids: ["u1"],
  categories: [{ id: "c1", name: "Hardware", is_default: true, queue_id: "q1" }],
};

function routes(modules: string[] = ["chat", "helpdesk"], extra: Routes = {}): Routes {
  return {
    ...chatRoutes,
    "GET /api/v1/meta": () => ({ name: "samnak-chat", version: "x", modules }),
    "GET /api/v1/chat/extensions": () => ({
      commands: [],
      message_actions: [
        { id: "helpdesk.create_from_message", module: "helpdesk" },
        { id: "task.create_from_message", module: "task" },
      ],
    }),
    "GET /api/v1/chat/channels": () => ({ items: [general] }),
    "GET /api/v1/chat/channels/c1": () => general,
    "GET /api/v1/chat/channels/c1/members": () => ({ items: [mali, somchai] }),
    "GET /api/v1/chat/channels/c1/messages": () => ({ items: [request], has_more: false }),
    "POST /api/v1/chat/channels/c1/read": () => undefined,
    "GET /api/v1/helpdesk/queues": () => ({ items: [itQueue] }),
    "POST /api/v1/helpdesk/tickets": () => ({ id: "t9", key: "IT-0143" }),
    ...extra,
  };
}

describe("create ticket from a message", () => {
  it("offers Create ticket on a message when /meta lists helpdesk", async () => {
    renderApp({ route: "/chat/c1", routes: routes() });
    const item = (await screen.findByText("The printer on floor 3 is jammed")).closest("li")!;
    expect(await within(item).findByRole("button", { name: "Create ticket" })).toBeInTheDocument();
  });

  it("hides Create ticket when /meta does not list helpdesk", async () => {
    renderApp({ route: "/chat/c1", routes: routes(["chat", "task"]) });
    const item = (await screen.findByText("The printer on floor 3 is jammed")).closest("li")!;
    // Create task is the check that the message actions have loaded.
    await within(item).findByRole("button", { name: "Create task" });
    expect(within(item).queryByRole("button", { name: "Create ticket" })).not.toBeInTheDocument();
  });

  it("opens the new-ticket dialog with the message text as its details", async () => {
    const user = userEvent.setup();
    renderApp({ route: "/chat/c1", routes: routes() });
    const item = (await screen.findByText("The printer on floor 3 is jammed")).closest("li")!;
    await user.click(await within(item).findByRole("button", { name: "Create ticket" }));
    const dialog = await screen.findByRole("dialog", { name: "New ticket" });
    await user.click(within(dialog).getByRole("button", { name: "More options" }));
    expect(within(dialog).getByRole("textbox", { name: "Details" })).toHaveValue("The printer on floor 3 is jammed");
  });

  it("shows a mention as the name in the details, since they are plain text", async () => {
    const user = userEvent.setup();
    const withMention = message("c1", 1, { body: "<@u2> the printer is jammed", sender_id: "u1" });
    renderApp({
      route: "/chat/c1",
      routes: routes(undefined, { "GET /api/v1/chat/channels/c1/messages": () => ({ items: [withMention], has_more: false }) }),
    });
    const item = (await screen.findByText("@Somchai")).closest("li")!;
    await user.click(await within(item).findByRole("button", { name: "Create ticket" }));
    const dialog = await screen.findByRole("dialog", { name: "New ticket" });
    await user.click(within(dialog).getByRole("button", { name: "More options" }));
    expect(within(dialog).getByRole("textbox", { name: "Details" })).toHaveValue("@Somchai the printer is jammed");
  });

  it("sends the ticket with the message as its source, then closes", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/chat/c1", routes: routes() });
    const item = (await screen.findByText("The printer on floor 3 is jammed")).closest("li")!;
    await user.click(await within(item).findByRole("button", { name: "Create ticket" }));
    const dialog = await screen.findByRole("dialog", { name: "New ticket" });
    await user.type(within(dialog).getByRole("textbox", { name: "Title" }), "Printer jammed");
    await user.click(within(dialog).getByRole("button", { name: "Send ticket" }));

    await waitFor(() => expect(app.called("POST /api/v1/helpdesk/tickets")).toHaveLength(1));
    expect(app.called("POST /api/v1/helpdesk/tickets")[0]!.body).toEqual({
      category_id: "c1",
      title: "Printer jammed",
      description: "The printer on floor 3 is jammed",
      priority: "normal",
      source_message_id: request.id,
    });
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "New ticket" })).not.toBeInTheDocument());
  });
});
