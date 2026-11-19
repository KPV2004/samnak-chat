import { useMutation } from "@tanstack/react-query";
import { Loader2Icon } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { useT } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { useMe } from "../state/queries";
import { useSession } from "../state/session";

/**
 * Nudges a signed-in user whose address is not verified yet. Sign-in is not blocked (AUTH-09),
 * so this only offers to send the mail again.
 */
export function VerifyEmailBanner() {
  const t = useT();
  const session = useSession();
  const me = useMe();
  const resend = useMutation({
    mutationFn: () => session.api.resendVerification(),
    onSuccess: () => toast.success(t("verify.resent")),
    onError: (err) => toast.error(t(errorKey(err))),
  });

  if (!me.data || me.data.email_verified) return null;

  return (
    <div className="flex flex-wrap items-center justify-between gap-2 border-b bg-muted px-4 py-2 text-sm">
      <p className="m-0">{t("verify.bannerBody", { email: me.data.email })}</p>
      <Button type="button" variant="outline" size="sm" onClick={() => resend.mutate()} disabled={resend.isPending}>
        {resend.isPending && <Loader2Icon className="animate-spin" aria-hidden="true" />}
        {t("verify.resend")}
      </Button>
    </div>
  );
}
