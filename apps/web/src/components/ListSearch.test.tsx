import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { render } from "@testing-library/react";
import { useLocale } from "../state/locale";
import { ListSearch } from "./ListSearch";

describe("ListSearch", () => {
  useLocale.setState({ locale: "en" });
  it("names the field with the loaded count and clears on Escape", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<ListSearch value="printer" onChange={onChange} count={20} />);
    const input = screen.getByRole("searchbox", { name: "Search the 20 loaded items" });
    expect(input).toHaveValue("printer");
    await user.type(input, "{Escape}");
    expect(onChange).toHaveBeenLastCalledWith("");
  });
});
