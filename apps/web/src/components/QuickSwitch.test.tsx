import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { channel, chatRoutes, renderApp } from "../test-utils";

const general = channel("c1", { name: "general" });
const dm = channel("c2", { type: "dm", name: "", dm_user_ids: ["u1", "u2"] });

const routes = {
  ...chatRoutes,
  "GET /api/v1/chat/channels": () => ({ items: [general, dm] }),
  "GET /api/v1/chat/channels/c1/messages": () => ({ items: [], has_more: false }),
  "GET /api/v1/chat/channels/c1": () => general,
  "GET /api/v1/chat/channels/c1/members": () => ({ items: [] }),
};

describe("search bar", () => {
  it("names the workspace and jumps to a channel", async () => {
    const user = userEvent.setup();
    renderApp({ route: "/members", routes });
    await user.click(await screen.findByRole("button", { name: "Search Acme" }));

    const dialog = await screen.findByRole("dialog", { name: "Jump to" });
    await user.type(await screen.findByPlaceholderText("Find a channel, person or page"), "gene");
    await user.click(await screen.findByRole("option", { name: /general/ }));

    expect(dialog).not.toBeInTheDocument();
    expect(await screen.findByRole("heading", { name: /general/ })).toBeInTheDocument();
  });

  it("opens on Ctrl+K and says when nothing matches", async () => {
    const user = userEvent.setup();
    renderApp({ route: "/", routes });
    await screen.findByRole("button", { name: "Search Acme" });
    await user.keyboard("{Control>}k{/Control}");
    await user.type(await screen.findByPlaceholderText("Find a channel, person or page"), "zzzz");
    expect(await screen.findByText("Nothing matches.")).toBeInTheDocument();
  });
});
