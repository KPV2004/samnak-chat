import { Skeleton } from "@/components/ui/skeleton";
import { AccountLoading } from "./AccountStates";

/** Shown in the shell while a page's code loads (see App.tsx). A heading and a block, like most pages. */
export function RouteFallback() {
  return (
    <AccountLoading>
      <Skeleton className="h-6 w-40" />
      <Skeleton className="h-24 w-full" />
    </AccountLoading>
  );
}
