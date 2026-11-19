import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { channel, renderApp, type Routes } from "../test-utils";

const general = channel("c1", { name: "general" });

function routes(modules: string[], prefs: Record<string, unknown> = {}, extra: Routes = {}): Routes {
  return {
    "GET /api/v1/meta": () => ({ name: "samnak-chat", version: "x", modules }),
    "GET /api/v1/chat/channels": () => ({ items: [general] }),
    "GET /api/v1/chat/channels/c1": () => general,
    "GET /api/v1/chat/channels/c1/members": () => ({ items: [] }),
    "GET /api/v1/chat/channels/c1/messages": () => ({ items: [], has_more: false }),
    "GET /api/v1/notify/preferences": () => ({
      disabled_kinds: [],
      muted_channel_ids: [],
      dnd_start: null,
      dnd_end: null,
      ...prefs,
    }),
    "PUT /api/v1/notify/preferences": (body) => body,
    ...extra,
  };
}

describe("muting a channel from its header", () => {
  it("mutes the channel and keeps the other settings", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/chat/c1",
      routes: routes(["chat", "notify"], { disabled_kinds: ["chat.dm"], muted_channel_ids: ["c9"] }),
    });
    await user.click(await screen.findByRole("button", { name: "Channel options" }));
    await user.click(await screen.findByRole("menuitem", { name: "Mute channel" }));

    expect(await screen.findByText("Channel muted")).toBeInTheDocument();
    expect(app.called("PUT /api/v1/notify/preferences")[0]!.body).toEqual({
      disabled_kinds: ["chat.dm"],
      muted_channel_ids: ["c9", "c1"],
      dnd_start: null,
      dnd_end: null,
    });
  });

  it("offers to unmute a channel that is already muted", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/chat/c1",
      routes: routes(["chat", "notify"], { muted_channel_ids: ["c1"] }),
    });
    await user.click(await screen.findByRole("button", { name: "Channel options" }));
    await user.click(await screen.findByRole("menuitem", { name: "Unmute channel" }));

    await waitFor(() => expect(app.called("PUT /api/v1/notify/preferences")).toHaveLength(1));
    expect(app.called("PUT /api/v1/notify/preferences")[0]!.body).toMatchObject({ muted_channel_ids: [] });
    expect(await screen.findByText("Channel unmuted")).toBeInTheDocument();
  });

  it("has no channel options while notify is not an enabled module", async () => {
    renderApp({ route: "/chat/c1", routes: routes(["chat"]) });
    await screen.findByRole("heading", { name: "general" });
    expect(screen.queryByRole("button", { name: "Channel options" })).not.toBeInTheDocument();
  });
});
