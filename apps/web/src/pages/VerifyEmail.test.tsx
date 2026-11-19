import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { errorRes, renderApp } from "../test-utils";

describe("VerifyEmail", () => {
  it("confirms the address from the link while signed out", async () => {
    const app = renderApp({
      route: "/verify-email?token=tok1",
      signedIn: false,
      routes: { "POST /api/v1/auth/email/verify": () => undefined },
    });

    expect(await screen.findByText("Your email is verified.")).toBeInTheDocument();
    const [call] = app.called("POST /api/v1/auth/email/verify");
    expect(call!.body).toEqual({ token: "tok1" });
    expect(call!.headers).not.toHaveProperty("Authorization");
    expect(screen.getByRole("link", { name: "Continue to Samnak Chat" })).toHaveAttribute("href", "/");
  });

  it("stays on the page for a signed-in user and sends no session token", async () => {
    const app = renderApp({
      route: "/verify-email?token=tok1",
      routes: { "POST /api/v1/auth/email/verify": () => undefined },
    });

    expect(await screen.findByText("Your email is verified.")).toBeInTheDocument();
    expect(app.called("POST /api/v1/auth/email/verify")[0]!.headers).not.toHaveProperty("Authorization");
  });

  it("says the link is invalid or expired when the server refuses it", async () => {
    const app = renderApp({
      route: "/verify-email?token=old",
      signedIn: false,
      routes: { "POST /api/v1/auth/email/verify": () => errorRes(400, "invalid") },
    });

    expect(await screen.findByText("This link is invalid or has expired. Sign in to get a new one.")).toBeInTheDocument();
    expect(screen.queryByText("Your email is verified.")).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Sign in" })).toHaveAttribute("href", "/login");
    expect(app.called("POST /api/v1/auth/email/verify")).toHaveLength(1);
  });

  it("treats a link with no token as invalid without calling the server", async () => {
    const app = renderApp({ route: "/verify-email", signedIn: false });

    expect(await screen.findByText("This link is invalid or has expired. Sign in to get a new one.")).toBeInTheDocument();
    expect(app.called("POST /api/v1/auth/email/verify")).toHaveLength(0);
  });

  it("offers a retry when the server cannot be reached, then confirms", async () => {
    let attempts = 0;
    const app = renderApp({
      route: "/verify-email?token=tok1",
      signedIn: false,
      routes: {
        "POST /api/v1/auth/email/verify": () => {
          attempts += 1;
          return attempts === 1 ? errorRes(500, "internal") : undefined;
        },
      },
    });

    expect(await screen.findByRole("alert")).toHaveTextContent("Something went wrong. Try again.");
    await userEvent.setup().click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByText("Your email is verified.")).toBeInTheDocument();
    expect(app.called("POST /api/v1/auth/email/verify")).toHaveLength(2);
  });
});
