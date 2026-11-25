import { fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { chatRoutes, renderApp, type Routes } from "../test-utils";

const meta = { "GET /api/v1/meta": () => ({ name: "samnak-chat", version: "x", modules: ["approval"] }) };

/** A request from Somchai (u2) waiting for me (u1) to approve. */
const waiting = {
  id: "r1",
  type: "general",
  status: "pending",
  payload: { subject: "Laptop purchase", details: "", currency: "THB" },
  requester_id: "u2",
  approver_id: "u1",
  message_id: null,
  reason: "",
  decided_at: null,
  created_at: "2026-11-17T09:00:00Z",
  updated_at: "2026-11-17T09:00:00Z",
};

function routes(extra: Routes = {}): Routes {
  return {
    ...chatRoutes,
    ...meta,
    "GET /api/v1/approval/requests": () => ({ items: [waiting], next_cursor: "" }),
    "POST /api/v1/approval/requests/r1/reject": () => ({}),
    "GET /api/v1/approval/requests/r1": () => waiting,
    ...extra,
  };
}

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("quick decisions from a Home card", () => {
  it("approves in place without opening the request, and shows the undo toast", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const app = renderApp({ route: "/", routes: routes() });

    fireEvent.click(await screen.findByRole("button", { name: "Approve Laptop purchase" }));

    expect(await screen.findByText("Approved · Undo within 5 seconds")).toBeInTheDocument();
    // BT-1: the button must not sit in the row's link, so the page stays on Home.
    expect(app.called("GET /api/v1/approval/requests/r1")).toHaveLength(0);
    expect(screen.getByRole("heading", { name: "Home" })).toBeInTheDocument();
    expect(screen.getByText("Waiting for you")).toBeInTheDocument();
  });

  it("opens the reject dialog from the card instead of opening the request (BT-2)", async () => {
    const app = renderApp({ route: "/", routes: routes() });

    fireEvent.click(await screen.findByRole("button", { name: "Reject Laptop purchase" }));

    expect(await screen.findByRole("dialog", { name: "Reject this request" })).toBeInTheDocument();
    expect(app.called("GET /api/v1/approval/requests/r1")).toHaveLength(0);

    fireEvent.change(screen.getByLabelText("Reason"), { target: { value: "Out of budget" } });
    fireEvent.click(screen.getByRole("button", { name: "Confirm rejection" }));

    await waitFor(() =>
      expect(app.called("POST /api/v1/approval/requests/r1/reject")[0]!.body).toEqual({ reason: "Out of budget" }),
    );
  });
});
