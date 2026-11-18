import { createHelpdeskApi, createTaskApi, type TaskApi } from "@samnak/core";
import { useMemo } from "react";
import { useSession } from "../state/session";

/** The helpdesk calls, on the session's authenticated client. */
export function useHelpdeskApi() {
  const session = useSession();
  return useMemo(() => createHelpdeskApi(session.client), [session]);
}

/** The task calls the ticket page makes (HD-19): core's task client, the same one the task module uses. */
export function useTaskApi(): TaskApi {
  const session = useSession();
  return useMemo(() => createTaskApi(session.client), [session]);
}

export interface PresignedUpload {
  url: string;
  method: string;
  headers: Record<string, string>;
  expires_at: string;
}

/**
 * The file calls of a ticket (HD-03): presign an upload, and read a download link. Core's helpdesk
 * client does not hold them yet, so they are called here on the same session client.
 */
export interface HelpdeskFileApi {
  requestUpload(input: { file_name: string; mime_type: string; size: number }): Promise<{ attachment_id: string; upload: PresignedUpload }>;
  attachmentUrl(id: string): Promise<{ url: string; expires_at: string }>;
}

const base = "/api/v1/helpdesk";
const enc = encodeURIComponent;

export function useHelpdeskFileApi(): HelpdeskFileApi {
  const session = useSession();
  return useMemo<HelpdeskFileApi>(
    () => ({
      requestUpload: (input) =>
        session.client.request(`${base}/uploads`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(input),
        }),
      attachmentUrl: (id) => session.client.request(`${base}/attachments/${enc(id)}`),
    }),
    [session],
  );
}

/**
 * Sends one file to storage through its presigned upload and returns the attachment id that the
 * ticket names. Null when storage refuses the file; the caller says so in its own words.
 */
export async function uploadAttachment(api: HelpdeskFileApi, file: File): Promise<string | null> {
  const upload = await api.requestUpload({
    file_name: file.name,
    mime_type: file.type || "application/octet-stream",
    size: file.size,
  });
  const sent = await fetch(upload.upload.url, { method: upload.upload.method, headers: upload.upload.headers, body: file });
  return sent.ok ? upload.attachment_id : null;
}
