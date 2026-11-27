import { Search, X } from "lucide-react";
import { Input } from "@/components/ui/input";
import { useT } from "@/i18n/useT";

/**
 * The search field of a list page. The API has no search endpoint, so it filters the rows already
 * loaded and says so in its label; `Load more` keeps loading and the new page is filtered too.
 */
export function ListSearch({ value, onChange, count }: { value: string; onChange: (v: string) => void; count: number }) {
  const t = useT();
  const label = t("list.search", { n: count });
  return (
    <div role="search" className="relative w-full max-w-sm">
      <Search aria-hidden="true" className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
      <Input
        type="search"
        aria-label={label}
        placeholder={label}
        value={value}
        className="pl-8 pr-8"
        onChange={(e) => onChange(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Escape") {
            e.preventDefault();
            onChange("");
          }
        }}
      />
      {value !== "" && (
        <button
          type="button"
          aria-label={t("list.search.clear")}
          onClick={() => onChange("")}
          className="absolute top-1/2 right-1.5 grid size-6 -translate-y-1/2 place-items-center rounded-sm text-muted-foreground outline-none hover:bg-accent focus-visible:ring-2 focus-visible:ring-ring"
        >
          <X aria-hidden="true" className="size-4" />
        </button>
      )}
    </div>
  );
}

/** Case-insensitive substring over any of the fields a row shows; an empty query matches all. */
export function matchesQuery(query: string, ...fields: (string | null | undefined)[]): boolean {
  const needle = query.trim().toLocaleLowerCase();
  if (needle === "") return true;
  return fields.some((f) => (f ?? "").toLocaleLowerCase().includes(needle));
}
