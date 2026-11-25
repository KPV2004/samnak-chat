import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { chatRoutes, renderApp, type Routes } from "../test-utils";

const meta = { "GET /api/v1/meta": () => ({ name: "samnak-chat", version: "x", modules: ["approval"] }) };
const create = "POST /api/v1/approval/requests";

function request(patch: Record<string, unknown>) {
  return {
    id: "r1",
    type: "leave",
    status: "pending",
    payload: { leave_type: "annual", start_date: "2026-11-20", end_date: "2026-11-21", reason: "Trip" },
    requester_id: "u1",
    approver_id: "u2",
    message_id: null,
    reason: "",
    decided_at: null,
    created_at: "2026-11-17T09:00:00Z",
    updated_at: "2026-11-17T09:00:00Z",
    ...patch,
  };
}

const mine = request({ id: "r1" });
const waiting = request({
  id: "r2",
  type: "general",
  payload: { subject: "Laptop purchase", details: "", currency: "THB" },
  requester_id: "u2",
  approver_id: "u1",
});

/** The two lists come from the one endpoint, told apart by `role`. */
function routes(extra: Routes = {}): Routes {
  return {
    ...chatRoutes,
    ...meta,
    "GET /api/v1/approval/requests": (_b, url) => ({
      items: url.includes("role=approver") ? [waiting] : [mine],
      next_cursor: "",
    }),
    "POST /api/v1/approval/requests/r2/reject": () => ({}),
    [create]: () => request({ id: "r9" }),
    ...extra,
  };
}

async function fillLeave(user: ReturnType<typeof userEvent.setup>, { start = "2026-11-20", end = "2026-11-20" } = {}) {
  await user.selectOptions(screen.getByRole("combobox", { name: "Leave type" }), "sick");
  fireEvent.change(screen.getByLabelText("Start date"), { target: { value: start } });
  fireEvent.change(screen.getByLabelText("End date"), { target: { value: end } });
  await user.type(screen.getByLabelText("Reason"), "Flu");
}

afterEach(() => vi.unstubAllGlobals());

describe("Approvals page", () => {
  it("lists the requests waiting for me by default, and only those", async () => {
    renderApp({ route: "/approvals", routes: routes() });
    const waitingList = await screen.findByRole("list", { name: "Waiting for me" });
    expect(within(waitingList).getByText("Laptop purchase")).toBeInTheDocument();
    expect(within(waitingList).getByText("Waiting")).toBeInTheDocument();
    expect(screen.queryByRole("list", { name: "My requests" })).not.toBeInTheDocument();
  });

  it("lists my own requests with ?view=mine, and only those", async () => {
    renderApp({ route: "/approvals?view=mine", routes: routes() });
    const mineList = await screen.findByRole("list", { name: "My requests" });
    expect(within(mineList).getByText("Annual leave · 2026-11-20 – 2026-11-21")).toBeInTheDocument();
    expect(screen.queryByRole("list", { name: "Waiting for me" })).not.toBeInTheDocument();
  });

  it("asks for pending requests only in the waiting list", async () => {
    const urls: string[] = [];
    const spy = routes({
      "GET /api/v1/approval/requests": (_b, url) => {
        urls.push(url);
        return { items: url.includes("role=approver") ? [waiting] : [mine], next_cursor: "" };
      },
    });
    renderApp({ route: "/approvals", routes: spy });
    await screen.findByRole("list", { name: "Waiting for me" });
    expect(urls.find((u) => u.includes("role=approver"))).toContain("status=pending");
    expect(urls.find((u) => u.includes("role=requester"))).toBeUndefined();
  });

  it("asks for all of my requests, whatever their status, in my list", async () => {
    const urls: string[] = [];
    renderApp({
      route: "/approvals?view=mine",
      routes: routes({
        "GET /api/v1/approval/requests": (_b, url) => {
          urls.push(url);
          return { items: [mine], next_cursor: "" };
        },
      }),
    });
    await screen.findByRole("list", { name: "My requests" });
    expect(urls.find((u) => u.includes("role=requester"))).not.toContain("status=");
  });

  it("says when a list is empty", async () => {
    const empty = routes({ "GET /api/v1/approval/requests": () => ({ items: [], next_cursor: "" }) });
    renderApp({ route: "/approvals?view=mine", routes: empty });
    expect(await screen.findByText("You have not sent any requests yet.")).toBeInTheDocument();
    cleanup();
    renderApp({ route: "/approvals", routes: empty });
    expect(await screen.findByText("No request is waiting for you.")).toBeInTheDocument();
  });

  it("opens the new request as a dialog, and Cancel closes it without sending", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/approvals?view=mine", routes: routes() });
    await screen.findByRole("list", { name: "My requests" });
    await user.click(screen.getByRole("button", { name: "New request" }));
    const dialog = await screen.findByRole("dialog", { name: "New request" });
    await user.type(within(dialog).getByLabelText("Reason"), "Flu");
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "New request" })).not.toBeInTheDocument());
    expect(app.called(create)).toHaveLength(0);
  });

  it("sends a leave request to the default approver", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/approvals?view=mine", routes: routes() });
    await screen.findByRole("list", { name: "My requests" });
    await user.click(screen.getByRole("button", { name: "New request" }));
    await fillLeave(user);
    await user.click(screen.getByRole("button", { name: "Send request" }));
    await waitFor(() => expect(app.called(create)).toHaveLength(1));
    expect(app.called(create)[0]!.body).toEqual({
      type: "leave",
      payload: { leave_type: "sick", start_date: "2026-11-20", end_date: "2026-11-20", reason: "Flu" },
      attachment_ids: [],
    });
    expect(await screen.findByText("Request sent")).toBeInTheDocument();
  });

  it("sends the approver the requester picked, and offers only other members", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/approvals?view=mine", routes: routes() });
    await screen.findByRole("list", { name: "My requests" });
    await user.click(screen.getByRole("button", { name: "New request" }));
    await user.click(await screen.findByRole("button", { name: "More options" }));
    const picker = screen.getByRole("combobox", { name: "Approver" });
    expect(within(picker).getAllByRole("option").map((o) => o.textContent)).toEqual(["Automatic: my group's manager", "Somchai"]);
    await user.selectOptions(picker, "u2");
    await fillLeave(user);
    await user.click(screen.getByRole("button", { name: "Send request" }));
    await waitFor(() => expect(app.called(create)).toHaveLength(1));
    expect(app.called(create)[0]!.body).toMatchObject({ approver_id: "u2" });
  });

  it("asks for an end date not before the start date, and sends nothing", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/approvals?view=mine", routes: routes() });
    await screen.findByRole("list", { name: "My requests" });
    await user.click(screen.getByRole("button", { name: "New request" }));
    await fillLeave(user, { start: "2026-11-20", end: "2026-11-18" });
    await user.click(screen.getByRole("button", { name: "Send request" }));
    expect(await screen.findByText("The end date cannot be before the start date")).toBeInTheDocument();
    expect(app.called(create)).toHaveLength(0);
  });

  it("allows a half day only on a single day", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/approvals?view=mine", routes: routes() });
    await screen.findByRole("list", { name: "My requests" });
    await user.click(screen.getByRole("button", { name: "New request" }));
    await fillLeave(user, { start: "2026-11-20", end: "2026-11-21" });
    await user.selectOptions(screen.getByRole("combobox", { name: "Half day" }), "morning");
    await user.click(screen.getByRole("button", { name: "Send request" }));
    expect(await screen.findByText("A half day needs a single day")).toBeInTheDocument();
    expect(app.called(create)).toHaveLength(0);
  });

  it("sends a general request with an amount and the default currency", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/approvals?view=mine", routes: routes() });
    await screen.findByRole("list", { name: "My requests" });
    await user.click(screen.getByRole("button", { name: "New request" }));
    await user.selectOptions(screen.getByRole("combobox", { name: "Type" }), "general");
    await user.type(screen.getByLabelText("Subject"), "Laptop purchase");
    await user.type(screen.getByLabelText("Details"), "For the design team");
    await user.type(screen.getByLabelText("Amount (optional)"), "1200.50");
    expect(screen.getByLabelText("Currency")).toHaveValue("THB");
    await user.click(screen.getByRole("button", { name: "Send request" }));
    await waitFor(() => expect(app.called(create)).toHaveLength(1));
    expect(app.called(create)[0]!.body).toEqual({
      type: "general",
      payload: { subject: "Laptop purchase", details: "For the design team", amount: "1200.50", currency: "THB" },
      attachment_ids: [],
    });
  });

  it("sends a general request without an amount and no currency, which the server would refuse", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/approvals?view=mine", routes: routes() });
    await screen.findByRole("list", { name: "My requests" });
    await user.click(screen.getByRole("button", { name: "New request" }));
    await user.selectOptions(screen.getByRole("combobox", { name: "Type" }), "general");
    await user.type(screen.getByLabelText("Subject"), "Team lunch");
    await user.click(screen.getByRole("button", { name: "Send request" }));
    await waitFor(() => expect(app.called(create)).toHaveLength(1));
    expect(app.called(create)[0]!.body).toEqual({
      type: "general",
      payload: { subject: "Team lunch", details: "" },
      attachment_ids: [],
    });
  });

  it("rejects an amount that is not a number", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/approvals?view=mine", routes: routes() });
    await screen.findByRole("list", { name: "My requests" });
    await user.click(screen.getByRole("button", { name: "New request" }));
    await user.selectOptions(screen.getByRole("combobox", { name: "Type" }), "general");
    await user.type(screen.getByLabelText("Subject"), "Laptop");
    await user.type(screen.getByLabelText("Amount (optional)"), "a lot");
    await user.click(screen.getByRole("button", { name: "Send request" }));
    expect(await screen.findByText("Use a number such as 1200.50")).toBeInTheDocument();
    expect(app.called(create)).toHaveLength(0);
  });

  it("uploads each file first, then sends its id with the request", async () => {
    const user = userEvent.setup();
    const put = vi.fn(async () => new Response(null, { status: 200 }));
    vi.stubGlobal("fetch", put);
    const app = renderApp({
      route: "/approvals?view=mine",
      routes: routes({
        "POST /api/v1/approval/uploads": () => ({
          attachment_id: "a1",
          upload: {
            url: "https://files.example/put?sig=1",
            method: "PUT",
            headers: { "Content-Type": "application/pdf" },
            expires_at: "2026-11-17T10:00:00Z",
          },
        }),
      }),
    });
    await screen.findByRole("list", { name: "My requests" });
    await user.click(screen.getByRole("button", { name: "New request" }));
    await user.selectOptions(screen.getByRole("combobox", { name: "Type" }), "general");
    await user.type(screen.getByLabelText("Subject"), "Laptop");
    await user.click(screen.getByRole("button", { name: "More options" }));
    const file = new File(["%PDF"], "quote.pdf", { type: "application/pdf" });
    await user.upload(screen.getByLabelText("Attachments"), file);
    expect(await screen.findByText("quote.pdf")).toBeInTheDocument();

    expect(app.called("POST /api/v1/approval/uploads")[0]!.body).toEqual({ file_name: "quote.pdf", mime_type: "application/pdf", size: 4 });
    expect(put).toHaveBeenCalledWith(
      "https://files.example/put?sig=1",
      expect.objectContaining({ method: "PUT", headers: { "Content-Type": "application/pdf" }, body: file }),
    );

    await user.click(screen.getByRole("button", { name: "Send request" }));
    await waitFor(() => expect(app.called(create)).toHaveLength(1));
    expect(app.called(create)[0]!.body).toMatchObject({ attachment_ids: ["a1"] });
  });

  it("removes an attached file before sending", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", vi.fn(async () => new Response(null, { status: 200 })));
    renderApp({
      route: "/approvals?view=mine",
      routes: routes({
        "POST /api/v1/approval/uploads": () => ({
          attachment_id: "a1",
          upload: { url: "https://files.example/put", method: "PUT", headers: {}, expires_at: "2026-11-17T10:00:00Z" },
        }),
      }),
    });
    await screen.findByRole("list", { name: "My requests" });
    await user.click(screen.getByRole("button", { name: "New request" }));
    await user.click(await screen.findByRole("button", { name: "More options" }));
    await user.upload(screen.getByLabelText("Attachments"), new File(["x"], "quote.pdf", { type: "application/pdf" }));
    await user.click(await screen.findByRole("button", { name: "Remove quote.pdf" }));
    expect(screen.queryByText("quote.pdf")).not.toBeInTheDocument();
  });

  it("opens the reject dialog from a list row and sends the reason (BT-2, AC-40)", async () => {
    const app = renderApp({ route: "/approvals", routes: routes() });

    fireEvent.click(await screen.findByRole("button", { name: "Reject Laptop purchase" }));

    expect(await screen.findByRole("dialog", { name: "Reject this request" })).toBeInTheDocument();
    // The row no longer sends the reader to the detail page.
    expect(app.called("GET /api/v1/approval/requests/r2")).toHaveLength(0);

    fireEvent.change(screen.getByLabelText("Reason"), { target: { value: "Not this quarter" } });
    fireEvent.click(screen.getByRole("button", { name: "Confirm rejection" }));

    await waitFor(() =>
      expect(app.called("POST /api/v1/approval/requests/r2/reject")[0]!.body).toEqual({ reason: "Not this quarter" }),
    );
  });
});
