import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { acme, chatRoutes, errorRes, renderApp, type Routes } from "../test-utils";

const groups = [
  { id: "g1", name: "Sales", manager_ids: ["u1"], member_count: 2 },
  { id: "g2", name: "Support", manager_ids: [], member_count: 0 },
];

const salesMembers = [
  { user_id: "u1", display_name: "Mali", is_manager: true },
  { user_id: "u2", display_name: "Somchai", is_manager: false },
];

function routes(role = "owner", extra: Routes = {}): Routes {
  return {
    ...chatRoutes,
    "GET /api/v1/workspace": () => ({ items: [{ ...acme, role }] }),
    "GET /api/v1/meta": () => ({ name: "samnak-chat", version: "x", modules: ["approval"] }),
    "GET /api/v1/workspace/groups": () => ({ items: groups }),
    "GET /api/v1/workspace/groups/g1/members": () => ({ items: salesMembers }),
    "GET /api/v1/workspace/groups/g2/members": () => ({ items: [] }),
    ...extra,
  };
}

describe("Groups page", () => {
  it("shows every group with its managers to a plain member, with no editing", async () => {
    const user = userEvent.setup();
    renderApp({ route: "/groups", routes: routes("member") });
    const list = await screen.findByRole("list", { name: "Groups" });
    expect(within(list).getByText("2 members")).toBeInTheDocument();
    expect(await within(list).findByText("Managers: Mali")).toBeInTheDocument();
    expect(screen.getByText("Only owners and admins can change groups.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Create group" })).not.toBeInTheDocument();

    await user.click(within(list).getByRole("button", { name: "Sales" }));
    const members = await screen.findByRole("list", { name: "Members" });
    expect(within(members).getByText("Mali")).toBeInTheDocument();
    expect(within(members).getByText("Manager")).toBeInTheDocument();
    expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Delete group" })).not.toBeInTheDocument();
  });

  it("keeps a workspace manager read-only, as the server does", async () => {
    renderApp({ route: "/groups", routes: routes("manager") });
    await screen.findByRole("list", { name: "Groups" });
    expect(screen.queryByRole("button", { name: "Create group" })).not.toBeInTheDocument();
  });

  it("creates a group for an owner", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/groups",
      routes: routes("owner", { "POST /api/v1/workspace/groups": () => ({ id: "g3", name: "Finance", manager_ids: [], member_count: 0 }) }),
    });
    await screen.findByRole("list", { name: "Groups" });
    await user.type(screen.getByLabelText("Group name"), "Finance");
    await user.click(screen.getByRole("button", { name: "Create group" }));
    await waitFor(() => expect(app.called("POST /api/v1/workspace/groups")).toHaveLength(1));
    expect(app.called("POST /api/v1/workspace/groups")[0]!.body).toEqual({ name: "Finance" });
  });

  it("explains a name that is already taken", async () => {
    const user = userEvent.setup();
    renderApp({
      route: "/groups",
      routes: routes("owner", { "POST /api/v1/workspace/groups": () => errorRes(409, "conflict") }),
    });
    await screen.findByRole("list", { name: "Groups" });
    await user.type(screen.getByLabelText("Group name"), "Sales");
    await user.click(screen.getByRole("button", { name: "Create group" }));
    expect(await screen.findByText("A group with this name already exists")).toBeInTheDocument();
  });

  it("makes a member a manager and saves the whole set in one request", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/groups",
      routes: routes("owner", { "PUT /api/v1/workspace/groups/g1/members": () => undefined }),
    });
    await user.click(within(await screen.findByRole("list", { name: "Groups" })).getByRole("button", { name: "Sales" }));
    expect(await screen.findByRole("checkbox", { name: "Mali" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "Manager: Mali" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "Somchai" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "Manager: Somchai" })).not.toBeChecked();

    await user.click(screen.getByRole("checkbox", { name: "Manager: Somchai" }));
    await user.click(screen.getByRole("button", { name: "Save members" }));
    await waitFor(() => expect(app.called("PUT /api/v1/workspace/groups/g1/members")).toHaveLength(1));
    expect(app.called("PUT /api/v1/workspace/groups/g1/members")[0]!.body).toEqual({
      members: [
        { user_id: "u1", is_manager: true },
        { user_id: "u2", is_manager: true },
      ],
    });
  });

  it("drops a member's manager role when they leave the group", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/groups",
      routes: routes("owner", { "PUT /api/v1/workspace/groups/g1/members": () => undefined }),
    });
    await user.click(within(await screen.findByRole("list", { name: "Groups" })).getByRole("button", { name: "Sales" }));
    await user.click(await screen.findByRole("checkbox", { name: "Mali" }));
    expect(screen.getByRole("checkbox", { name: "Manager: Mali" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "Save members" }));
    await waitFor(() => expect(app.called("PUT /api/v1/workspace/groups/g1/members")).toHaveLength(1));
    expect(app.called("PUT /api/v1/workspace/groups/g1/members")[0]!.body).toEqual({
      members: [{ user_id: "u2", is_manager: false }],
    });
  });

  it("renames a group", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/groups",
      routes: routes("owner", { "PATCH /api/v1/workspace/groups/g1": (b) => ({ ...groups[0], ...b }) }),
    });
    await user.click(within(await screen.findByRole("list", { name: "Groups" })).getByRole("button", { name: "Sales" }));
    const rename = await screen.findByLabelText("Rename to");
    await user.clear(rename);
    await user.type(rename, "Sales team");
    await user.click(screen.getByRole("button", { name: "Save name" }));
    await waitFor(() => expect(app.called("PATCH /api/v1/workspace/groups/g1")).toHaveLength(1));
    expect(app.called("PATCH /api/v1/workspace/groups/g1")[0]!.body).toEqual({ name: "Sales team" });
  });

  it("deletes a group only after a confirmation", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/groups",
      routes: routes("owner", { "DELETE /api/v1/workspace/groups/g1": () => undefined }),
    });
    await user.click(within(await screen.findByRole("list", { name: "Groups" })).getByRole("button", { name: "Sales" }));
    await user.click(await screen.findByRole("button", { name: "Delete group" }));
    expect(app.called("DELETE /api/v1/workspace/groups/g1")).toHaveLength(0);
    await user.click(screen.getByRole("button", { name: "Confirm deleting Sales" }));
    await waitFor(() => expect(app.called("DELETE /api/v1/workspace/groups/g1")).toHaveLength(1));
  });
});
