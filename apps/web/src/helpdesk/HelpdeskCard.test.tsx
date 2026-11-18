import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { channel, chatRoutes, message, renderApp, type Routes } from "../test-utils";

// Mali (u1) is the signed-in user and an agent of the IT queue (q1); Somchai (u2) is the requester.
const ticket = {
  ticket_id: "t1",
  key: "IT-0142",
  title: "Printer on fire",
  status: "new",
  priority: "high",
  queue_name: "IT",
  requester_id: "u2",
  requester_name: "Somchai",
  assignee_id: null,
  assignee_name: "",
  can_claim_queue_id: "q1",
  version: 1,
};

const general = channel("c1", { name: "general", last_seq: 1 });
const assign = "POST /api/v1/helpdesk/tickets/t1/assign";
const queuesKey = "GET /api/v1/helpdesk/queues";
const itQueue = { id: "q1", name: "IT", prefix: "IT", channel_id: "c1", agent_ids: ["u1"], categories: [] };
const hrQueue = { id: "q2", name: "HR", prefix: "HR", channel_id: null, agent_ids: ["u9"], categories: [] };

function ticketMessage(payload: object, body = "Ticket IT-0142: Printer on fire (new)") {
  return message("c1", 1, { kind: "helpdesk.ticket", body: "", fallback_text: body, payload });
}

function routes(messages: object[], extra: Routes = {}, modules = ["helpdesk"]): Routes {
  return {
    ...chatRoutes,
    "GET /api/v1/meta": () => ({ name: "samnak-chat", version: "x", modules }),
    "GET /api/v1/chat/channels": () => ({ items: [general] }),
    "GET /api/v1/chat/channels/c1": () => general,
    "GET /api/v1/chat/channels/c1/members": () => ({ items: [] }),
    "GET /api/v1/chat/channels/c1/messages": () => ({ items: messages, has_more: false }),
    "POST /api/v1/chat/channels/c1/read": () => undefined,
    [queuesKey]: () => ({ items: [itQueue, hrQueue] }),
    ...extra,
  };
}

describe("helpdesk.ticket card", () => {
  it("shows the key, title, queue and requester, with an Open link to the ticket", async () => {
    renderApp({ route: "/chat/c1", routes: routes([ticketMessage(ticket)]) });
    expect(await screen.findByText("Printer on fire")).toBeInTheDocument();
    expect(screen.getByText("IT-0142")).toBeInTheDocument();
    expect(screen.getByText("IT · from Somchai")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open" })).toHaveAttribute("href", "/helpdesk/t1");
  });

  it("lets an agent of the queue claim an unassigned ticket", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/chat/c1", routes: routes([ticketMessage(ticket)], { [assign]: () => ({}) }) });
    await user.click(await screen.findByRole("button", { name: "Claim" }));
    await waitFor(() => expect(app.called(assign)).toHaveLength(1));
    expect(app.called(assign)[0]!.body).toEqual({ assignee_id: "u1" });
  });

  it("offers Claim only on a card of a queue the viewer works, and not on someone else's", async () => {
    const mine = ticketMessage(ticket);
    const theirs = ticketMessage({ ...ticket, ticket_id: "t2", key: "HR-0007", title: "Leave form", queue_name: "HR", can_claim_queue_id: "q2" });
    renderApp({ route: "/chat/c1", routes: routes([mine, theirs]) });
    const mineItem = (await screen.findByText("Printer on fire")).closest("li")!;
    const theirsItem = (await screen.findByText("Leave form")).closest("li")!;
    // The Claim on my card shows only once the queue list has loaded, so the other card's absence is checked after that.
    await within(mineItem).findByRole("button", { name: "Claim" });
    expect(within(theirsItem).queryByRole("button", { name: "Claim" })).not.toBeInTheDocument();
  });

  it("does not offer Claim once the ticket has an assignee", async () => {
    const assigned = ticketMessage({ ...ticket, ticket_id: "t3", key: "IT-0150", title: "Lost laptop", assignee_id: "u2", assignee_name: "Somchai", status: "open" });
    renderApp({ route: "/chat/c1", routes: routes([ticketMessage(ticket), assigned]) });
    const assignedItem = (await screen.findByText("Lost laptop")).closest("li")!;
    const mineItem = (await screen.findByText("Printer on fire")).closest("li")!;
    await within(mineItem).findByRole("button", { name: "Claim" });
    expect(within(assignedItem).queryByRole("button", { name: "Claim" })).not.toBeInTheDocument();
  });

  it("shows the new status and assignee when the server sends card.updated", async () => {
    const m = ticketMessage(ticket);
    const app = renderApp({ route: "/chat/c1", routes: routes([m]) });
    await screen.findByRole("button", { name: "Claim" });
    const socket = await app.live();
    act(() =>
      socket.receive("card.updated", {
        channel_id: "c1",
        message_id: m.id,
        payload: { ...ticket, status: "in_progress", assignee_id: "u1", assignee_name: "Mali", version: 2 },
      }),
    );
    expect(await screen.findByText("In progress")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Claim" })).not.toBeInTheDocument();
  });

  it("shows its fallback text when /meta lacks helpdesk", async () => {
    renderApp({ route: "/chat/c1", routes: routes([ticketMessage(ticket)], {}, []) });
    expect(await screen.findByText("Ticket IT-0142: Printer on fire (new)")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Open" })).not.toBeInTheDocument();
  });
});
