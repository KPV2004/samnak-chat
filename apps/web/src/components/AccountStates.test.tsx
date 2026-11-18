import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useLocale } from "../state/locale";
import { AccountLoadError, AccountLoading } from "./AccountStates";

describe("AccountLoading", () => {
  beforeEach(() => useLocale.setState({ locale: "en" }));

  it("announces the loading text and draws the shape it stands in for", () => {
    render(
      <AccountLoading>
        <div data-testid="shape" />
      </AccountLoading>,
    );
    expect(screen.getByRole("status")).toHaveTextContent("Loading…");
    expect(screen.getByTestId("shape")).toBeInTheDocument();
  });
});

describe("AccountLoadError", () => {
  beforeEach(() => useLocale.setState({ locale: "en" }));

  it("is an alert with the message and a retry that calls back", async () => {
    const user = userEvent.setup();
    const onRetry = vi.fn();
    render(<AccountLoadError message="Could not load" onRetry={onRetry} />);
    expect(screen.getByRole("alert")).toHaveTextContent("Could not load");
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it("has no retry button when there is nothing to retry", () => {
    render(<AccountLoadError message="Could not load" />);
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });
});
