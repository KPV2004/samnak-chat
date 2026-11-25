import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { render } from "@testing-library/react";
import { useLocale } from "../state/locale";
import { MoreOptions } from "./MoreOptions";

describe("MoreOptions", () => {
  useLocale.setState({ locale: "en" });
  it("keeps the optional fields collapsed until asked", async () => {
    const user = userEvent.setup();
    render(
      <MoreOptions>
        <input aria-label="Optional field" />
      </MoreOptions>,
    );
    expect(screen.queryByLabelText("Optional field")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "More options" }));
    expect(screen.getByLabelText("Optional field")).toBeInTheDocument();
  });
});
