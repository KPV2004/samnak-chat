import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { acme, chatRoutes, renderApp, type Routes } from "../test-utils";

const itQueue = {
  id: "q1",
  name: "IT",
  prefix: "IT",
  channel_id: null,
  agent_ids: [],
  categories: [{ id: "c1", name: "Hardware", is_default: true, queue_id: "q1" }],
};
const createKey = "POST /api/v1/helpdesk/tickets";
const uploadKey = "POST /api/v1/helpdesk/uploads";
const putUrl = "https://files.example/put?sig=1";

function routes(extra: Routes = {}): Routes {
  return {
    ...chatRoutes,
    "GET /api/v1/meta": () => ({ name: "samnak-chat", version: "x", modules: ["helpdesk"] }),
    "GET /api/v1/workspace": () => ({ items: [{ ...acme, role: "member" }] }),
    "GET /api/v1/helpdesk/queues": () => ({ items: [itQueue] }),
    "GET /api/v1/helpdesk/tickets": () => ({ items: [], next_cursor: "" }),
    [uploadKey]: () => ({
      attachment_id: "a1",
      upload: { url: putUrl, method: "PUT", headers: { "Content-Type": "application/pdf" }, expires_at: "2026-11-17T10:00:00Z" },
    }),
    [createKey]: () => ({ id: "t9", key: "IT-0143" }),
    ...extra,
  };
}

async function openDialog(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole("button", { name: "New ticket" }));
  const dialog = await screen.findByRole("dialog", { name: "New ticket" });
  await user.click(within(dialog).getByRole("button", { name: "More options" }));
  return dialog;
}

describe("attachments in the new-ticket dialog", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("uploads each file first, then sends its id with the ticket", async () => {
    const user = userEvent.setup();
    const put = vi.fn(async () => new Response(null, { status: 200 }));
    vi.stubGlobal("fetch", put);
    const app = renderApp({ route: "/helpdesk", routes: routes() });
    const dialog = await openDialog(user);

    const file = new File(["%PDF"], "quote.pdf", { type: "application/pdf" });
    await user.upload(within(dialog).getByLabelText("Attachments"), file);
    expect(await within(dialog).findByText("quote.pdf")).toBeInTheDocument();

    expect(app.called(uploadKey)[0]!.body).toEqual({ file_name: "quote.pdf", mime_type: "application/pdf", size: 4 });
    expect(put).toHaveBeenCalledWith(putUrl, expect.objectContaining({ method: "PUT", headers: { "Content-Type": "application/pdf" }, body: file }));

    await user.type(within(dialog).getByRole("textbox", { name: "Title" }), "Printer jammed");
    await user.click(within(dialog).getByRole("button", { name: "Send ticket" }));
    await waitFor(() => expect(app.called(createKey)).toHaveLength(1));
    expect(app.called(createKey)[0]!.body).toMatchObject({ attachment_ids: ["a1"] });
  });

  it("removes an attached file before sending, and sends no attachment ids", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", vi.fn(async () => new Response(null, { status: 200 })));
    const app = renderApp({ route: "/helpdesk", routes: routes() });
    const dialog = await openDialog(user);
    await user.upload(within(dialog).getByLabelText("Attachments"), new File(["x"], "quote.pdf", { type: "application/pdf" }));
    await user.click(await within(dialog).findByRole("button", { name: "Remove quote.pdf" }));
    expect(within(dialog).queryByText("quote.pdf")).not.toBeInTheDocument();

    await user.type(within(dialog).getByRole("textbox", { name: "Title" }), "Printer jammed");
    await user.click(within(dialog).getByRole("button", { name: "Send ticket" }));
    await waitFor(() => expect(app.called(createKey)).toHaveLength(1));
    expect(app.called(createKey)[0]!.body).not.toHaveProperty("attachment_ids");
  });

  it("refuses a file over 50 MB without uploading it", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/helpdesk", routes: routes() });
    const dialog = await openDialog(user);
    const big = new File(["x"], "scan.pdf", { type: "application/pdf" });
    Object.defineProperty(big, "size", { value: 51 * 1024 * 1024 });
    await user.upload(within(dialog).getByLabelText("Attachments"), big);
    expect(await within(dialog).findByText("scan.pdf is larger than 50 MB")).toBeInTheDocument();
    expect(app.called(uploadKey)).toHaveLength(0);
  });

  it("starts without the files of an earlier, closed dialog", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", vi.fn(async () => new Response(null, { status: 200 })));
    renderApp({ route: "/helpdesk", routes: routes() });
    const dialog = await openDialog(user);
    await user.upload(within(dialog).getByLabelText("Attachments"), new File(["x"], "quote.pdf", { type: "application/pdf" }));
    await within(dialog).findByText("quote.pdf");
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());

    const reopened = await openDialog(user);
    expect(within(reopened).queryByText("quote.pdf")).not.toBeInTheDocument();
  });
});
