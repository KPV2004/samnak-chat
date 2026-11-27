import { screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { channel, chatRoutes, message, renderApp, type Routes } from "../test-utils";

const room = channel("c1", { name: "general", last_seq: 1, last_read_seq: 1 });

function routes(extra: Routes = {}): Routes {
  return {
    ...chatRoutes,
    "GET /api/v1/chat/channels": () => ({ items: [room] }),
    "GET /api/v1/chat/channels/c1": () => room,
    "GET /api/v1/chat/channels/c1/members": () => ({ items: [] }),
    "GET /api/v1/chat/channels/c1/messages": () => ({ items: [message("c1", 1)], has_more: false }),
    "POST /api/v1/chat/channels/c1/read": () => undefined,
    ...extra,
  };
}

afterEach(() => vi.restoreAllMocks());

describe("opening a channel", () => {
  it("renders the message list and the composer under distinct keys (BT-4)", async () => {
    const errors = vi.spyOn(console, "error").mockImplementation(() => {});
    renderApp({ route: "/chat/c1", routes: routes() });

    await screen.findByRole("log", { name: "Messages" });
    expect(await screen.findByRole("textbox", { name: "Message" })).toBeInTheDocument();

    const duplicateKeys = errors.mock.calls.filter((call) => String(call[0]).includes("same key"));
    expect(duplicateKeys).toEqual([]);
  });
});
