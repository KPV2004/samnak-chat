import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { channel, chatRoutes, errorRes, message, renderApp, type Routes } from "../test-utils";

// Mali (u1, the signed-in user) is the approver; Somchai (u2) is the requester.
const card = {
  request_id: "r1",
  type: "leave",
  status: "pending",
  summary: "Annual leave · 2026-11-20 · 2026-11-21",
  fields: {},
  requester_id: "u2",
  requester_name: "Somchai",
  approver_id: "u1",
  approver_name: "Mali",
  reason: "",
  decided_at: null,
  version: 1,
};

const general = channel("c1", { name: "general", last_seq: 1 });
const approve = "POST /api/v1/approval/requests/r1/approve";
const reject = "POST /api/v1/approval/requests/r1/reject";
const cancel = "POST /api/v1/approval/requests/r1/cancel";

function routes(payload: object, extra: Routes = {}, modules = ["approval"]): Routes {
  return {
    ...chatRoutes,
    "GET /api/v1/meta": () => ({ name: "samnak-chat", version: "x", modules }),
    "GET /api/v1/chat/channels": () => ({ items: [general] }),
    "GET /api/v1/chat/channels/c1": () => general,
    "GET /api/v1/chat/channels/c1/members": () => ({ items: [] }),
    "GET /api/v1/chat/channels/c1/messages": () => ({
      items: [message("c1", 1, { kind: "approval.request", body: "", fallback_text: "Leave request from Somchai", payload })],
      has_more: false,
    }),
    "POST /api/v1/chat/channels/c1/read": () => undefined,
    ...extra,
  };
}

describe("approval.request card", () => {
  it("offers approve and reject to the named approver while pending", async () => {
    renderApp({ route: "/chat/c1", routes: routes(card) });
    expect(await screen.findByText("Annual leave · 2026-11-20 · 2026-11-21")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Approve" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Reject" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Cancel request" })).not.toBeInTheDocument();
  });

  it("approves from the card", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/chat/c1", routes: routes(card, { [approve]: () => undefined }) });
    await user.click(await screen.findByRole("button", { name: "Approve" }));
    await waitFor(() => expect(app.called(approve)).toHaveLength(1));
  });

  it("needs a reason to reject, then sends it", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/chat/c1", routes: routes(card, { [reject]: () => ({}) }) });
    await user.click(await screen.findByRole("button", { name: "Reject" }));
    await user.click(screen.getByRole("button", { name: "Confirm rejection" }));
    expect(screen.getByText("Give a reason for rejecting")).toBeInTheDocument();
    expect(app.called(reject)).toHaveLength(0);

    await user.type(screen.getByRole("textbox", { name: "Reason" }), "Clashes with the audit");
    await user.click(screen.getByRole("button", { name: "Confirm rejection" }));
    await waitFor(() => expect(app.called(reject)[0]!.body).toEqual({ reason: "Clashes with the audit" }));
  });

  it("lets the requester cancel while it is pending", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/chat/c1",
      routes: routes({ ...card, requester_id: "u1", approver_id: "u2", requester_name: "Mali", approver_name: "Somchai" }, { [cancel]: () => undefined }),
    });
    await user.click(await screen.findByRole("button", { name: "Cancel request" }));
    await waitFor(() => expect(app.called(cancel)).toHaveLength(1));
    expect(screen.queryByRole("button", { name: "Approve" })).not.toBeInTheDocument();
  });

  it("shows the reason of a rejection and no actions once decided", async () => {
    renderApp({
      route: "/chat/c1",
      routes: routes({ ...card, status: "rejected", reason: "Clashes with the audit", decided_at: "2026-11-17T09:00:00Z" }),
    });
    expect(await screen.findByText("Reason: Clashes with the audit")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Approve" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Reject" })).not.toBeInTheDocument();
  });

  it("explains a decision that was already made", async () => {
    const user = userEvent.setup();
    renderApp({ route: "/chat/c1", routes: routes(card, { [approve]: () => errorRes(409, "conflict") }) });
    await user.click(await screen.findByRole("button", { name: "Approve" }));
    expect(await screen.findByText("This request was already decided.")).toBeInTheDocument();
  });

  it("shows its fallback text when /meta lacks approval", async () => {
    renderApp({ route: "/chat/c1", routes: routes(card, {}, []) });
    expect(await screen.findByText("Leave request from Somchai")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Approve" })).not.toBeInTheDocument();
  });
});
