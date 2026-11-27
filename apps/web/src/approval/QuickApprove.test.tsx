import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { chatRoutes, errorRes, renderApp, type Routes } from "../test-utils";

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
    ...extra,
  };
}

/** The one row in the waiting list, whose badge shows the request's status. */
async function waitingRow() {
  const list = await screen.findByRole("list", { name: "Waiting for me" });
  return within(list).getByRole("listitem");
}

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("quick approve from a list", () => {
  it("shows the decision at once but sends nothing until the grace ends, and Undo cancels the send", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const app = renderApp({ route: "/approvals", routes: routes() });
    let row = await waitingRow();
    expect(within(row).getByText("Waiting")).toBeInTheDocument();

    fireEvent.click(within(row).getByRole("button", { name: "Approve Laptop purchase" }));

    // The row shows the decision before the request is sent.
    await waitFor(() => expect(within(row).getByText("Approved")).toBeInTheDocument());
    expect(app.called("POST /api/v1/approval/requests/r1/approve")).toHaveLength(0);

    // Undo within the grace puts the row back and never sends.
    fireEvent.click(await screen.findByRole("button", { name: "Undo" }));
    await waitFor(() => expect(within(row).getByText("Waiting")).toBeInTheDocument());
    await act(async () => {
      await vi.advanceTimersByTimeAsync(6_000);
    });
    row = await waitingRow();
    expect(within(row).getByText("Waiting")).toBeInTheDocument();
    expect(app.called("POST /api/v1/approval/requests/r1/approve")).toHaveLength(0);
  });

  it("sends after the grace, and a 409 puts the row back and says it was already decided", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const app = renderApp({
      route: "/approvals",
      routes: routes({
        "POST /api/v1/approval/requests/r1/approve": () => errorRes(409, "conflict", "already decided"),
      }),
    });
    const row = await waitingRow();
    fireEvent.click(within(row).getByRole("button", { name: "Approve Laptop purchase" }));
    await waitFor(() => expect(within(row).getByText("Approved")).toBeInTheDocument());

    await act(async () => {
      await vi.advanceTimersByTimeAsync(5_100);
    });

    await waitFor(() => expect(app.called("POST /api/v1/approval/requests/r1/approve")).toHaveLength(1));
    expect(await screen.findByText("This request was already decided.")).toBeInTheDocument();
    // The optimistic status is rolled back to what the server still holds.
    const list = await screen.findByRole("list", { name: "Waiting for me" });
    await waitFor(() => expect(within(list).getByText("Waiting")).toBeInTheDocument());
    expect(within(list).queryByText("Approved")).not.toBeInTheDocument();
  });
});
