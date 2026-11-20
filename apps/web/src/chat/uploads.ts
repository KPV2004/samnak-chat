import { useMemo } from "react";
import { useSession } from "../state/session";

/** The most the server takes in one file (`POST /chat/uploads`). */
export const maxFileBytes = 50 * 1024 * 1024;

interface Presigned {
  attachment_id: string;
  upload: { url: string; method: string; headers: Record<string, string> };
}

/**
 * Sends one file to a channel's storage: asks for a presigned upload, then puts the file there.
 * Returns the attachment id to name in `attachment_ids` when the message is sent. Throws when
 * either step is refused.
 */
export function useChannelUpload(): (channelId: string, file: File) => Promise<string> {
  const session = useSession();
  return useMemo(
    () => async (channelId, file) => {
      const mime = file.type || "application/octet-stream";
      const presigned = await session.client.request<Presigned>("/api/v1/chat/uploads", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ channel_id: channelId, file_name: file.name, mime_type: mime, size: file.size }),
      });
      const put = await fetch(presigned.upload.url, { method: presigned.upload.method, headers: presigned.upload.headers, body: file });
      if (!put.ok) throw new Error("upload refused");
      return presigned.attachment_id;
    },
    [session],
  );
}
