import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { useLocale } from "../state/locale";
import { errorRes, expectAlert, me, renderApp } from "../test-utils";

describe("Profile", () => {
  it("saves name, language and time zone, and switches the interface language", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/profile",
      routes: { "PATCH /api/v1/auth/me": (b) => ({ ...me, ...b }) },
    });
    const name = await screen.findByLabelText("Display name");
    expect(name).toHaveValue("Mali");
    await user.clear(name);
    await user.type(name, "Mali S.");
    await user.selectOptions(screen.getByLabelText("Preferred language"), "th");
    await user.clear(screen.getByLabelText("Time zone"));
    await user.type(screen.getByLabelText("Time zone"), "Europe/Paris");
    await user.click(screen.getByRole("button", { name: "Save" }));

    // the interface switched to Thai as soon as the save succeeded
    expect(await screen.findByText("บันทึกแล้ว")).toBeInTheDocument();
    expect(app.called("PATCH /api/v1/auth/me")[0]!.body).toEqual({
      display_name: "Mali S.",
      locale: "th",
      timezone: "Europe/Paris",
    });
    expect(useLocale.getState().locale).toBe("th");
  });

  it("rejects an unknown time zone", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/profile" });
    const zone = await screen.findByLabelText("Time zone");
    await user.clear(zone);
    await user.type(zone, "Mars/Olympus");
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(zone).toHaveAccessibleDescription(/Unknown time zone/);
    expect(app.called("PATCH /api/v1/auth/me")).toHaveLength(0);
  });
});

describe("Devices", () => {
  const devices = [
    { id: "d1", name: "Chrome (web)", platform: "web", last_seen_at: "2026-11-01T10:00:00Z", current: true },
    { id: "d2", name: "Firefox (web)", platform: "web", last_seen_at: "2026-10-30T10:00:00Z", current: false },
  ];

  it("lists devices and revokes another one, but not the current one", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/devices",
      routes: {
        "GET /api/v1/auth/devices": () => ({ items: devices }),
        "DELETE /api/v1/auth/devices/d2": () => undefined,
      },
    });
    const current = (await screen.findByText("Chrome (web)")).closest("li")!;
    expect(within(current).getByText("This device")).toBeInTheDocument();
    expect(within(current).queryByRole("button")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Revoke device Firefox (web)" }));
    expect(app.called("DELETE /api/v1/auth/devices/d2")).toHaveLength(1);
    expect(await screen.findByText("Device signed out")).toBeInTheDocument();
  });

  it("retries when the list fails to load", async () => {
    const user = userEvent.setup();
    let attempts = 0;
    renderApp({
      route: "/devices",
      routes: {
        "GET /api/v1/auth/devices": () => (++attempts === 1 ? errorRes(500, "internal") : { items: devices }),
      },
    });
    await expectAlert("Something went wrong");
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByText("Chrome (web)")).toBeInTheDocument();
    expect(attempts).toBe(2);
  });

  it("says so when there are no devices", async () => {
    renderApp({ route: "/devices", routes: { "GET /api/v1/auth/devices": () => ({ items: [] }) } });
    expect(await screen.findByText("No signed-in devices")).toBeInTheDocument();
  });

  it("signs out of all devices and returns to the login page", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/devices",
      routes: { "GET /api/v1/auth/devices": () => ({ items: devices }), "POST /api/v1/auth/logout": () => undefined },
    });
    await screen.findByText("Chrome (web)");
    await user.click(screen.getByRole("button", { name: "Sign out of all devices" }));
    expect(await screen.findByRole("heading", { name: "Sign in" })).toBeInTheDocument();
    expect(app.called("POST /api/v1/auth/logout")[0]!.body).toEqual({ all_devices: true });
    expect(app.store.load()).toBeNull();
  });
});
