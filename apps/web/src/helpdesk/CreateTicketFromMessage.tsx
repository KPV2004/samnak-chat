import type { Message } from "@samnak/core";
import { useQuery } from "@tanstack/react-query";
import { useEffect } from "react";
import { toast } from "sonner";
import { toReadable } from "../chat/mentions";
import { useNameOf } from "../chat/names";
import { useT } from "../i18n/useT";
import { useSessionState } from "../state/session";
import { useHelpdeskApi } from "./api";
import { NewTicketDialog } from "./NewTicketDialog";

interface Props {
  /** The message the ticket is made from: its text becomes the details and its ID the source (HD-03). */
  message: Message;
  onDone(): void;
}

/**
 * The "Create ticket" message action (HD-03). It opens the new-ticket form with the message's text and
 * sends `source_message_id`, so the server copies the message's attachments onto the ticket.
 */
export function CreateTicketFromMessage({ message, onDone }: Props) {
  const t = useT();
  const api = useHelpdeskApi();
  const nameOf = useNameOf();
  const { workspaceId } = useSessionState();
  const queues = useQuery({
    queryKey: ["helpdesk", "queues", workspaceId],
    queryFn: () => api.listQueues(),
    enabled: workspaceId !== null,
  });
  const failed = queues.isError;

  // Without the queues there is no form to show, so say why and close.
  useEffect(() => {
    if (!failed) return;
    toast.error(t("helpdesk.loadError"));
    onDone();
  }, [failed, onDone, t]);

  if (!queues.data) return null;
  // Details are plain text, so mentions show as names here, as the task form shows them.
  const details = toReadable(message.body, nameOf).text.trim();
  return (
    <NewTicketDialog
      open
      onOpenChange={(open) => {
        if (!open) onDone();
      }}
      queues={queues.data.items}
      initialDescription={details}
      sourceMessageId={message.id}
    />
  );
}
