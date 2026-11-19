import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { errorRes, renderApp } from "../test-utils";

describe("ResetPassword", () => {
  it("sets the new password with the token from the link, signed out", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/reset-password?token=tok1",
      signedIn: false,
      routes: { "POST /api/v1/auth/password/reset": () => undefined },
    });

    await user.type(screen.getByLabelText("Password"), "correct horse");
    await user.click(screen.getByRole("button", { name: "Save new password" }));

    expect(await screen.findByRole("heading", { name: "Your password has been changed" })).toBeInTheDocument();
    expect(screen.getByText("Every device has been signed out. Sign in with your new password.")).toBeInTheDocument();
    const [call] = app.called("POST /api/v1/auth/password/reset");
    expect(call!.body).toEqual({ token: "tok1", password: "correct horse" });
    expect(call!.headers).not.toHaveProperty("Authorization");
    expect(screen.getByRole("link", { name: "Sign in" })).toHaveAttribute("href", "/login");
  });

  it("ends the local session of a user who is signed in on this browser", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/reset-password?token=tok1",
      routes: {
        "POST /api/v1/auth/password/reset": () => undefined,
        "POST /api/v1/auth/logout": () => undefined,
      },
    });

    await user.type(screen.getByLabelText("Password"), "correct horse");
    await user.click(screen.getByRole("button", { name: "Save new password" }));

    expect(await screen.findByRole("heading", { name: "Your password has been changed" })).toBeInTheDocument();
    await waitFor(() => expect(app.store.load()).toBeNull());
  });

  it("checks the password length before calling the server", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/reset-password?token=tok1", signedIn: false });

    await user.type(screen.getByLabelText("Password"), "short");
    await user.click(screen.getByRole("button", { name: "Save new password" }));

    const password = screen.getByLabelText("Password");
    expect(password).toBeInvalid();
    expect(password).toHaveAccessibleDescription(expect.stringContaining("The password must be 8 to 128 characters"));
    expect(app.called("POST /api/v1/auth/password/reset")).toHaveLength(0);
  });

  it("explains that a used or expired link cannot be used and offers a new one", async () => {
    const user = userEvent.setup();
    renderApp({
      route: "/reset-password?token=old",
      signedIn: false,
      routes: { "POST /api/v1/auth/password/reset": () => errorRes(400, "invalid") },
    });

    await user.type(screen.getByLabelText("Password"), "correct horse");
    await user.click(screen.getByRole("button", { name: "Save new password" }));

    expect(await screen.findByText("This link is invalid or has expired.")).toBeInTheDocument();
    expect(screen.queryByLabelText("Password")).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Request a new link" })).toHaveAttribute("href", "/forgot-password");
  });

  it("shows the invalid state at once when the link has no token", () => {
    const app = renderApp({ route: "/reset-password", signedIn: false });

    expect(screen.getByText("This link is invalid or has expired.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Request a new link" })).toHaveAttribute("href", "/forgot-password");
    expect(app.called("POST /api/v1/auth/password/reset")).toHaveLength(0);
  });
});
