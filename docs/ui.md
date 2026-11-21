# Web UI

How the web app (`apps/web`) looks and how to build a page in it. The look is Slack's: a narrow dark workspace rail, one dark sidebar per app, a dark top bar with a search-style bar, and the page on a light (or, in dark mode, dark) surface. Dense and calm, one green accent. The stack is Tailwind v4 and shadcn/ui (Radix); the shadcn components are copied into `apps/web/src/components/ui` and are ours to edit.

## Where things are

| Path | What |
| --- | --- |
| `src/index.css` | The only stylesheet: fonts, Tailwind, tokens, the `on-rail` scope and the base rules (focus ring, reduced motion, body) |
| `src/components/ui/` | shadcn components. Import as `@/components/ui/button` |
| `src/components/` | App components built on them: `Page`, `Field`, `FormError`, `EmptyState`, `AccountStates` (loading and load error), `RouteFallback`, `NavItem`, `UserAvatar`, `ThemeToggle`, `AppSidebar` (and one sidebar per app), `WorkspaceRail`, `AccountMenu`, `SidebarLink`, `SidebarSection`, `QuickSwitch`, `AuthLayout` |
| `src/pages/Shell.tsx` | The frame: top bar, workspace rail, sidebar, content. Below `md` the rail and the sidebar are one Sheet |
| `src/state/theme.ts` | Light, dark or system, kept in `localStorage` (`samnak.theme`) |
| `src/i18n/ui-messages.ts` | Strings of the frame and shared components |

`@/` means `src/`. Use it for `components/ui` and `lib/utils`; relative imports are fine elsewhere.

## Tokens

Colours are CSS variables in `index.css` (oklch, light in `:root`, dark in `.dark`) and are used as Tailwind classes. Never write a colour value in a component. The palette was dealt by the design skill (`start.mjs deal --page software`, palette "mint-pink-accounts": off-white ground, near-black ink, green accent `#1f6b3a`); the type pairing is Host Grotesk with IBM Plex Sans Thai for Thai. There is no violet anywhere.

| Class | Use |
| --- | --- |
| `bg-background` `text-foreground` | Page and body text (off-white, near-black; dark mode a green-tinted charcoal) |
| `bg-card` `text-card-foreground` | Cards, panels (white; a step lighter than the page in dark mode) |
| `bg-popover` | Menus, popovers, dialogs (the components set it) |
| `bg-primary` `text-primary-foreground` | The one accent (green): the main button, links, focus ring. Once per view |
| `bg-secondary` | Quiet filled buttons |
| `bg-muted` | Skeletons, code, subtle fills |
| `bg-accent` | Hover and selected state of menu items and ghost buttons |
| `text-muted-foreground` | Secondary text, hints, timestamps, icons |
| `border-border` `border-input` | Dividers and card borders, form control borders (default border colour is already `border-border`) |
| `text-destructive` `bg-destructive` | Errors and delete |
| `text-success` `text-warning` | Status only (approved, pending). Never decoration |
| `bg-rail` | The workspace rail and the top bar (the darkest chrome) |
| `bg-sidebar` `text-sidebar-*` | The sidebar, a step lighter than the rail. `sidebar-primary` (mint) fills the current row; `sidebar-accent` tints a hovered row |
| `ring-ring` | Focus. The components already add it; custom controls need `focus-visible:ring-[3px] focus-visible:ring-ring/50` |

The dark chrome looks the same in light and dark mode. Put the `on-rail` class on a container in it (the rail, the top bar, the sidebar, the mobile sheet) and everything inside re-reads the neutral tokens as light-on-dark: a `Button variant="ghost"`, a `Badge`, `text-muted-foreground` and the focus ring all work there with no `dark:` class. Portalled content (menus, tooltips, dialogs) sits outside the scope and keeps the page's tokens.

### One system

- **Buttons:** one pair. `default` (green, the main action, once per view) and `outline` (everything else); `ghost` for toolbars and icon buttons, `destructive` for delete. Every button steps its colour on hover, darkens a step on press and shrinks 3% while pressed (only without `prefers-reduced-motion`).
- **Card:** one style. White (`bg-card`), `rounded-lg`, a 1px `border`. No shadow, no nested cards.
- **Radius:** one scale. `rounded-sm` 4px (chips, keys), `rounded-md` 6px (controls, rows), `rounded-lg` 8px (cards, dialogs, workspace tiles), `rounded-xl` 12px (large panels), `rounded-full` (badges, avatars). `--radius` in `index.css` moves the whole scale.
- **Link:** one look. `text-primary underline-offset-4 hover:underline`. Rows in the sidebar are not text links: they tint on hover and press, and fill with mint while current.
- **Feedback:** every button, nav row and linked row answers the pointer (tint on hover, a step darker on press). Workspace tiles also lift 1px. All movement sits behind `motion-safe:`, and `prefers-reduced-motion: reduce` sets every transition and animation to about zero, so state changes still show but nothing moves.
- **Focus:** a 2 to 3px ring in `--ring` on every control, never removed. On the dark chrome the ring is light mint. The first Tab stop is the skip link.

Dark mode is the `dark` class on `<html>`. `dark:` works in classes, but tokens already switch, so you rarely need it. The toggle (`ThemeToggle`, in the top bar and on the auth pages) follows the system until the user picks, then remembers the pick.

## The shell

Top to bottom, left to right (`Shell.tsx`):

- **Top bar** (`bg-rail`, 44px): the product mark and name (name from `md`), the search-style bar (`QuickSwitch`: it names the workspace and opens a "jump to" list of channels, direct messages and pages, also on Ctrl or Cmd + K; it finds places, not message text), the notification bell, the language select (from `md`) and the theme toggle.
- **Workspace rail** (`WorkspaceRail`, 64px): a tile per workspace with its initials (the current one is filled mint and marked `aria-current`), clicking one switches workspace; a dashed tile creates a new one. Under them the apps, each an icon with its name: Messenger, Tasks, Approvals, Helpdesk (each only while its module is enabled) and Workspace. The current app is filled mint and marked `aria-current` (`appOf(pathname)` in `components/apps.ts` maps a path to its app). Messenger carries its unread count; Approvals carries the waiting count while that list is loaded (the rail reads the cache and makes no request of its own). At the foot, the signed-in user's avatar opens the **account menu** (`AccountMenu`): Profile, Devices, Notification settings (while notify is on), Theme, Language and Sign out.
- **Sidebar** (`AppSidebar`, 256px): shows only the app the rail has picked, under a header with the app's name and the workspace. `/` opens Messenger.
  - **Messenger** (`/chat/*`): the conversation list (CH-02), which has its own heading.
  - **Tasks** (`/tasks`, `TaskSidebar`): views My tasks (open), Due today, Overdue and Done, then a folding "By channel" list of the channels that hold my open tasks, with counts. Each is a link whose query string sets the page's filters: `status` (comma list; empty means all; absent means open: To do and Doing), `due` and `channel`. The page's toolbar reads and writes the same parameters (`task/view.ts`).
  - **Approvals** (`/approvals`, `ApprovalSidebar`): a "New request" button (`?new=1` opens the dialog), then Waiting for me (with its count) and My requests (`?view=mine`). The page shows one list at a time.
  - **Helpdesk** (`/helpdesk/*`, `HelpdeskSidebar`): a "New ticket" button (`?new=1`), My requests, then for agents and admins a folding Queue section with All queues (`?view=queue`) and one entry per queue the user is an agent of (`&queue=<id>`), and Settings for owners and admins. The page's tabs and queue filter follow the URL.
  - **Workspace** (`/members`, `/invites`, `/groups`, `/audit`, `WorkspaceSidebar`): one link each; Groups while approval is on, Audit log for owners and admins.
  - **Account** (`/profile`, `/devices`, `/notifications/settings`, `AccountSidebar`): the three account pages. No app is current on the rail there.
  - A row that points at a view uses `SidebarLink` (the router's `NavLink` only compares paths, so the sidebar says which view is current); a row that points at a page uses `NavItem`. Both tint on hover, dim on press and fill mint while current. `SidebarSection` folds a list inside the app it belongs to (open or closed is kept in `localStorage` as `samnak.sidebar.<id>`).
- **Below `md`:** the rail and the current app's sidebar are one Sheet from the menu button; it closes after a link or a workspace is chosen. The language select of the top bar is hidden there; the account menu has it.

The chat state is provided by the shell, so Messenger's unread count on the rail is visible on every page. Member names (needed to title direct messages) are fetched only once the user has a direct message.

## Type and spacing

- Font: Host Grotesk (Latin, variable, from `@fontsource-variable/host-grotesk`) with IBM Plex Sans Thai (`@fontsource`, weights 400, 500 and 600) behind it. Host Grotesk has no Thai, so the browser falls through to Plex glyph by glyph; both are self-hosted. Do not use `font-bold`; stay at 400, 500 and 600.
- Body is `text-sm` (14px) with line height 1.6, which Thai needs for tone marks. Do not add `leading-none`, `leading-tight` or `tracking-*` to text that can be Thai.
- Page title `text-xl font-semibold` (`Page` does this). Section title `text-base font-semibold`. Secondary text `text-sm text-muted-foreground`. Captions `text-xs`.
- Spacing is on the 4px grid. Page padding `p-4 md:p-6`, gap between page sections `gap-6`, between fields `gap-4`, label to control `gap-1.5`, inside toolbars and rows `gap-2`.
- Controls are `h-9`; use the `sm` size (`h-8`) in toolbars and dense rows.
- Lay out with `grid` or `flex` and `gap`, not margins on children.
- Icons are lucide, `size-4` in text and buttons. An icon next to a label is decoration; an icon alone needs an accessible name.

## Which component for what

| Need | Use |
| --- | --- |
| Main action of the view | `Button` (default). One per view |
| Other actions | `variant="outline"`; toolbar and icon buttons `ghost`; delete `destructive` and ask to confirm first |
| Icon-only button | `size="icon-sm"` plus `aria-label`, wrapped in `Tooltip` |
| A link that looks like a button | `<Button asChild><Link to="…">…</Link></Button>` |
| Text, email, password, date | `Field` (label, `Input`, hint and error wired together). Form-level error: `FormError` |
| Several lines | `Textarea` inside a `Label` |
| On/off, pick several | `Checkbox` with a `Label` |
| Pick one of a few | `NativeSelect` (see below). `Select` only for rich options |
| Pick from many, searchable | `Command` (in a `Popover`) |
| Menu of actions on one item, account menu | `DropdownMenu` |
| Small panel next to a control | `Popover` |
| Confirm, or a short form that needs full focus | `Dialog` |
| Side panel, mobile navigation | `Sheet` |
| Switch between views of the same data | `Tabs` |
| Table of records | `Table` |
| List of simple rows | `<ul className="divide-y rounded-lg border">` with `li` rows `px-4 py-3` |
| An object with its own actions, or a grouped section | `Card` (`CardHeader`, `CardTitle`, `CardContent`, `CardFooter`). Do not nest cards |
| Status, count, role | `Badge` (`secondary` neutral, `default` accent, `destructive`; success and warning as `outline` with `text-success` or `text-warning`) |
| A section of an app sidebar that folds | `SidebarSection` with `SidebarLink` or `NavItem` rows |
| Person | `UserAvatar` next to the name |
| Divider | `Separator` |
| Long area with its own scroll | `ScrollArea` |
| Loading | `AccountLoading` with `Skeleton` blocks shaped like the content (it adds the `role="status"` text) |
| Empty list or page | `EmptyState` (what is missing, and the action that fills it) |
| Load error | `AccountLoadError` (`role="alert"` text, and a `Button variant="outline"` to retry when given `onRetry`) |
| Result of an action that stays on the page ("Saved", "Copied") | `toast.success(t("…"))` from `sonner`. The `Toaster` is in the shell |

Errors that belong to a form stay in the form (`FormError`, `Field error`). Toasts are for confirmations and for failures with nowhere else to go; they disappear, so never put the only copy of an important message in one.

`NativeSelect` is a real `<select>`, so tests use `user.selectOptions` and phones show their own picker. Existing tests do that in many places, so keep it for them. Wrap it in `Field` with the render prop: `<Field label="Role">{({ id, describedBy }) => <NativeSelect id={id} aria-describedby={describedBy} containerClassName="w-full">…</NativeSelect>}</Field>`.

## Building a page

Start the page from `Page`; it gives the column, the title (`h1`), the description and the action slot.

```tsx
<Page title={t("members.title")} actions={<Button>…</Button>}>
  …
</Page>
```

Bare HTML has no default look: Tailwind's reset removes paragraph margins, list bullets and link underlines. Give every `p`, `ul`, `a` and `h1` its classes. Content outside a `Page` (a loading or gate state) sits in the shell's column, which gives it the page's width and padding; a `Page` brings its own, and the shell drops its padding when one is inside.

`Page` supports `width` (`narrow` forms, `default`, `wide` tables). Chat is full height and does not use it.

## Tests and shadcn in jsdom

- `test-setup.ts` stubs `ResizeObserver`, `scrollIntoView` and pointer capture, which Radix needs. Open menus and dialogs with `user.click`; options render in a portal on `document.body`.
- The sidebar shows the current app and the workspace name; the user's name and email are in the account menu (open it with `getByRole("button", { name: "Account menu" })`, then pick a `menuitem`; Sign out is there). A query like `findByText("Acme")` can match twice. Scope to the page with `within(screen.getByRole("main"))`. `/` redirects to `/chat`, so a test that needs another app's sidebar starts on that app's route.
- The desktop rail and sidebar are always in the DOM in tests (CSS does not run); the mobile Sheet is not until it is opened. Switch workspace by clicking its tile (`getByRole("button", { name: "Beta" })`), not with a select. Sidebar section state is in `localStorage`, so tests that fold one clear it first. In jsdom, open a menu's submenu (Theme, Language) with the keyboard (focus the item, `{ArrowRight}`, then `{Enter}` on the choice); a pointer click does not reach it.

## Adding a shadcn component

Run `npx shadcn@latest add <name>` from `apps/web`. The CLI still adds an npm package called `cn` and imports from `"cn"`. Fix it every time: `npm uninstall cn`, then change `from "cn"` to `from "@/lib/utils"` in the new file. Replace any hard-coded English text (such as "Close") with a `useT()` string, and add the string to `ui-messages.ts` in both languages.

## Checklist for a page

- [ ] Starts from `Page` and builds from `components/ui`. No new `.css` file, no inline `style`.
- [ ] Colours, radius and spacing come from tokens and the scale above. No hex or `oklch` in components.
- [ ] Every string goes through `useT()` and exists in both Thai and English. Long Thai text wraps (no fixed widths on text, no `truncate` on content people need to read).
- [ ] Looks right in dark mode (open it with the toggle) and at 390px wide. Tables scroll inside their own box; nothing makes the page scroll sideways.
- [ ] Accessible names are unchanged, so the page's tests still find what they looked for: same labels, same button text, same `role="alert"` and `role="status"` regions. Add `aria-label` to icon-only buttons.
- [ ] Keyboard: every action reachable and in a sensible order, visible focus, dialogs close with Escape.
- [ ] Module-gated parts stay gated (`useModuleEnabled`, `RequireModule`).
- [ ] Behaviour is unchanged: same requests, same state. A test changes only when its query depended on markup that is gone, and it still checks the same thing.
- [ ] `npm run build`, `npm test` and `npm run lint` pass from the repo root, with no skipped tests and no lint suppressions.
