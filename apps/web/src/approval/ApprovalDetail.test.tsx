import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { chatRoutes, errorRes, renderApp, type Routes } from "../test-utils";

// Mali (u1) is the approver of r1; Somchai (u2) is the requester.
const detail = {
  id: "r1",
  type: "leave",
  status: "pending",
  payload: { leave_type: "annual", start_date: "2026-11-20", end_date: "2026-11-21", half_day: "morning", reason: "Family trip" },
  requester_id: "u2",
  approver_id: "u1",
  message_id: null,
  reason: "",
  decided_at: null,
  created_at: "2026-11-17T09:00:00Z",
  updated_at: "2026-11-17T09:00:00Z",
  attachments: [{ id: "f1", file_name: "plan.pdf", mime_type: "application/pdf", size: 1200, created_at: "2026-11-17T09:00:00Z" }],
  history: [{ id: "h1", action: "created", actor_id: "u2", at: "2026-11-17T09:00:00Z", reason: "" }],
};

const decided = {
  ...detail,
  status: "rejected",
  reason: "Clashes with the audit",
  decided_at: "2026-11-17T10:00:00Z",
  history: [
    ...detail.history,
    { id: "h2", action: "rejected", actor_id: "u1", at: "2026-11-17T10:00:00Z", reason: "Clashes with the audit" },
  ],
};

const path = "/api/v1/approval/requests/r1";
const approve = `POST ${path}/approve`;

function routes(item: object, extra: Routes = {}): Routes {
  return {
    ...chatRoutes,
    "GET /api/v1/meta": () => ({ name: "samnak-chat", version: "x", modules: ["approval"] }),
    [`GET ${path}`]: () => item,
    ...extra,
  };
}

afterEach(() => vi.restoreAllMocks());

describe("approval detail", () => {
  it("shows the fields and the history, with names and reasons", async () => {
    renderApp({ route: "/approvals/r1", routes: routes(decided) });
    expect(await screen.findByRole("heading", { name: "Annual leave · 2026-11-20 – 2026-11-21" })).toBeInTheDocument();
    expect(screen.getByText("Family trip")).toBeInTheDocument();
    expect(screen.getByText("Morning")).toBeInTheDocument();

    const history = screen.getByRole("list", { name: "History" });
    expect(await within(history).findByText("Somchai sent the request")).toBeInTheDocument();
    expect(within(history).getByText("Mali rejected it")).toBeInTheDocument();
    expect(within(history).getByText("Clashes with the audit")).toBeInTheDocument();
  });

  it("offers approve and reject to the approver while pending", async () => {
    const app = renderApp({ route: "/approvals/r1", routes: routes(detail, { [approve]: () => undefined }) });
    await screen.findByRole("heading", { name: /Annual leave/ });
    expect(screen.getByRole("button", { name: "Reject" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Cancel request" })).not.toBeInTheDocument();
    await userEvent.setup().click(screen.getByRole("button", { name: "Approve" }));
    await waitFor(() => expect(app.called(approve)).toHaveLength(1));
  });

  it("opens an attachment through a fresh download link", async () => {
    const open = vi.spyOn(window, "open").mockReturnValue(null);
    const app = renderApp({
      route: "/approvals/r1",
      routes: routes(decided, {
        "GET /api/v1/approval/attachments/f1": () => ({ url: "https://files.example/plan.pdf?sig=2", expires_at: "2026-11-17T11:00:00Z" }),
      }),
    });
    await userEvent.setup().click(await screen.findByRole("button", { name: "Download plan.pdf" }));
    await waitFor(() => expect(open).toHaveBeenCalledWith("https://files.example/plan.pdf?sig=2", "_blank", "noopener"));
    expect(app.called("GET /api/v1/approval/attachments/f1")).toHaveLength(1);
  });

  it("says when the request does not exist or is not visible to me", async () => {
    renderApp({ route: "/approvals/r1", routes: routes(detail, { [`GET ${path}`]: () => errorRes(404, "not_found") }) });
    expect(await screen.findByText("This request does not exist, or you cannot see it.")).toBeInTheDocument();
  });
});
