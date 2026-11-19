import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { errorRes, renderApp } from "../test-utils";

describe("ForgotPassword", () => {
  it("asks for the email and does not call the server when it is empty", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/forgot-password", signedIn: false });

    await user.click(screen.getByRole("button", { name: "Send link" }));

    const email = screen.getByLabelText("Email");
    expect(email).toBeInvalid();
    expect(email).toHaveAccessibleDescription("Enter your email");
    expect(email).toHaveFocus();
    expect(app.called("POST /api/v1/auth/password/forgot")).toHaveLength(0);
  });

  it("sends the link request and says so without revealing whether the account exists", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/forgot-password",
      signedIn: false,
      routes: { "POST /api/v1/auth/password/forgot": () => undefined },
    });

    await user.type(screen.getByLabelText("Email"), "me@example.com");
    await user.click(screen.getByRole("button", { name: "Send link" }));

    expect(await screen.findByRole("heading", { name: "Check your email" })).toBeInTheDocument();
    expect(
      screen.getByText("If an account uses this email, we have sent a link to set a new password. The link works for one hour."),
    ).toBeInTheDocument();
    const [call] = app.called("POST /api/v1/auth/password/forgot");
    expect(call!.body).toEqual({ email: "me@example.com" });
    expect(call!.headers).not.toHaveProperty("Authorization");
    expect(screen.getByRole("link", { name: "Back to sign in" })).toHaveAttribute("href", "/login");
  });

  it("tells the user to wait when rate limited", async () => {
    const user = userEvent.setup();
    renderApp({
      route: "/forgot-password",
      signedIn: false,
      routes: { "POST /api/v1/auth/password/forgot": () => errorRes(429, "rate_limited") },
    });

    await user.type(screen.getByLabelText("Email"), "me@example.com");
    await user.click(screen.getByRole("button", { name: "Send link" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("Too many attempts");
    expect(screen.getByRole("heading", { name: "Forgot your password?" })).toBeInTheDocument();
  });
});
