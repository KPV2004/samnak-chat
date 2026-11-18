# Web UI

How the web app (`apps/web`) looks and how to build a page in it. The look is Slack/Linear: dense, quiet neutral colours, one accent, a left sidebar and a top bar, light and dark. The stack is Tailwind v4 and shadcn/ui (Radix); the shadcn components are copied into `apps/web/src/components/ui` and are ours to edit.

## Where things are

| Path | What |
| --- | --- |
| `src/index.css` | The only stylesheet: font, Tailwind, tokens and the base rules (focus ring, body) |
| `src/components/ui/` | shadcn components. Import as `@/components/ui/button` |
| `src/components/` | App components built on them: `Page`, `Field`, `FormError`, `EmptyState`, `AccountStates` (loading and load error), `RouteFallback`, `NavItem`, `UserAvatar`, `ThemeToggle`, `AppSidebar`, `AuthLayout` |
| `src/pages/Shell.tsx` | The frame: sidebar, top bar, content. Below `md` the sidebar is a Sheet |
| `src/state/theme.ts` | Light, dark or system, kept in `localStorage` (`samnak.theme`) |
| `src/i18n/ui-messages.ts` | Strings of the frame and shared components |

`@/` means `src/`. Use it for `components/ui` and `lib/utils`; relative imports are fine elsewhere.

## Tokens

Colours are CSS variables in `index.css` (oklch, light in `:root`, dark in `.dark`) and are used as Tailwind classes. Never write a colour value in a component.

| Class | Use |
| --- | --- |
| `bg-background` `text-foreground` | Page and body text |
| `bg-card` `text-card-foreground` | Cards, panels |
| `bg-popover` | Menus, popovers, dialogs (the components set it) |
| `bg-primary` `text-primary-foreground` | The one accent (indigo): the main button, current item, focus ring, links. Once per view |
| `bg-secondary` | Quiet filled buttons |
| `bg-muted` | Skeletons, code, subtle fills |
| `bg-accent` | Hover and selected state of menu items and ghost buttons |
| `text-muted-foreground` | Secondary text, hints, timestamps, icons |
| `border-border` `border-input` | Dividers and card borders, form control borders (default border colour is already `border-border`) |
| `text-destructive` `bg-destructive` | Errors and delete |
| `text-success` `text-warning` | Status only (approved, pending). Never decoration |
| `bg-sidebar` `text-sidebar-*` | Only the app sidebar |
| `ring-ring` | Focus. The components already add it; custom controls need `focus-visible:ring-[3px] focus-visible:ring-ring/50` |

Radius is `rounded-md` for controls and `rounded-lg` for cards and dialogs.

Dark mode is the `dark` class on `<html>`. `dark:` works in classes, but tokens already switch, so you rarely need it. The toggle (`ThemeToggle`, in the top bar and on the auth pages) follows the system until the user picks, then remembers the pick.

## Type and spacing

- Font: IBM Plex Sans Thai (Thai and Latin), self-hosted through `@fontsource`, weights 400, 500 and 600. Do not use `font-bold`.
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
- The sidebar shows the user's name and email, the workspace names and (in chat) the channels. A query like `findByText("Mali")` can now match twice. Scope to the page with `within(screen.getByRole("main"))`.
- The desktop sidebar is always in the DOM in tests (CSS does not run); the mobile Sheet is not until it is opened.

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
