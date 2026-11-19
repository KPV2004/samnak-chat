import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { acme, channel, errorRes, expectAlert, renderApp, type Routes } from "../test-utils";

const general = channel("c1", { name: "general" });
const allModules = ["chat", "notify", "task", "helpdesk", "approval"];

interface Prefs {
  disabled_kinds: string[];
  muted_channel_ids: string[];
  dnd_start: string | null;
  dnd_end: string | null;
}

const saved: Prefs = { disabled_kinds: ["chat.dm"], muted_channel_ids: ["c1"], dnd_start: null, dnd_end: null };

function routes(modules = allModules, prefs: Partial<Prefs> = {}, extra: Routes = {}): Routes {
  return {
    "GET /api/v1/meta": () => ({ name: "samnak-chat", version: "x", modules }),
    "GET /api/v1/workspace": () => ({ items: [{ ...acme, role: "member" }] }),
    "GET /api/v1/chat/channels": () => ({ items: [general] }),
    "GET /api/v1/notify/preferences": () => ({ ...saved, ...prefs }),
    "PUT /api/v1/notify/preferences": (body) => body,
    ...extra,
  };
}

// The page is lazy-loaded, so its first render can take longer than the default wait on a busy machine.
async function page() {
  return screen.findByRole("checkbox", { name: "Direct messages" }, { timeout: 3000 });
}

const puts = (app: { called: (key: string) => { body: Record<string, unknown> }[] }) =>
  app.called("PUT /api/v1/notify/preferences");

describe("Notification settings", () => {
  it("shows a switch for each kind of the enabled modules, with the saved ones", async () => {
    renderApp({ route: "/notifications/settings", routes: routes() });
    expect(await page()).not.toBeChecked();
    expect(screen.getByRole("checkbox", { name: "Mentions of you" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "Overdue tasks" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "Approval requests" })).toBeChecked();
  });

  it("hides the kinds of a module that is off", async () => {
    renderApp({ route: "/notifications/settings", routes: routes(["chat", "notify"]) });
    await page();
    expect(screen.queryByRole("checkbox", { name: "Overdue tasks" })).not.toBeInTheDocument();
    expect(screen.queryByRole("checkbox", { name: "Approval requests" })).not.toBeInTheDocument();
  });

  it("saves the switches and the do-not-disturb window together", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/notifications/settings", routes: routes() });
    await page();
    await user.click(screen.getByRole("checkbox", { name: "Direct messages" }));
    fireEvent.change(screen.getByLabelText("From"), { target: { value: "22:00" } });
    fireEvent.change(screen.getByLabelText("Until"), { target: { value: "07:00" } });
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(await screen.findByText("Notification settings saved")).toBeInTheDocument();
    expect(puts(app)[0]!.body).toEqual({
      disabled_kinds: [],
      muted_channel_ids: ["c1"],
      dnd_start: "22:00",
      dnd_end: "07:00",
    });
  });

  it("asks for both ends of the window and sends nothing", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/notifications/settings", routes: routes() });
    await page();
    fireEvent.change(screen.getByLabelText("From"), { target: { value: "22:00" } });
    await user.click(screen.getByRole("button", { name: "Save" }));

    await expectAlert("Set both times, or clear both.");
    expect(puts(app)).toHaveLength(0);
  });

  it("refuses a window whose start and end are the same time", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/notifications/settings", routes: routes() });
    await page();
    fireEvent.change(screen.getByLabelText("From"), { target: { value: "08:00" } });
    fireEvent.change(screen.getByLabelText("Until"), { target: { value: "08:00" } });
    await user.click(screen.getByRole("button", { name: "Save" }));

    await expectAlert("The start and the end cannot be the same time.");
    expect(puts(app)).toHaveLength(0);
  });

  it("turns the window off with both ends empty", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/notifications/settings",
      routes: routes(allModules, { dnd_start: "22:00", dnd_end: "07:00" }),
    });
    await page();
    expect(screen.getByLabelText("From")).toHaveValue("22:00");
    await user.click(screen.getByRole("button", { name: "Turn off" }));
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(puts(app)).toHaveLength(1));
    expect(puts(app)[0]!.body).toMatchObject({ dnd_start: null, dnd_end: null });
  });

  it("unmutes a channel at once and keeps the rest of the settings", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/notifications/settings",
      routes: routes(allModules, { dnd_start: "22:00", dnd_end: "07:00" }),
    });
    await page();
    const list = screen.getByRole("list", { name: "Muted channels" });
    expect(within(list).getByText("general")).toBeInTheDocument();
    await user.click(within(list).getByRole("button", { name: "Unmute general" }));

    await waitFor(() => expect(puts(app)).toHaveLength(1));
    expect(puts(app)[0]!.body).toEqual({
      disabled_kinds: ["chat.dm"],
      muted_channel_ids: [],
      dnd_start: "22:00",
      dnd_end: "07:00",
    });
  });

  it("says when the settings could not be saved", async () => {
    const user = userEvent.setup();
    renderApp({
      route: "/notifications/settings",
      routes: routes(allModules, {}, { "PUT /api/v1/notify/preferences": () => errorRes(500, "internal") }),
    });
    await page();
    await user.click(screen.getByRole("button", { name: "Save" }));
    await expectAlert("Something went wrong. Try again.");
  });

  it("is linked from the sidebar while notify is enabled", async () => {
    renderApp({ route: "/", routes: routes() });
    expect(await screen.findByRole("link", { name: "Notification settings" })).toHaveAttribute(
      "href",
      "/notifications/settings",
    );
  });
});
