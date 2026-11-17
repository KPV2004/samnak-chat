// Chat extensions (CHAT-16): the slash commands and message actions other
// modules register with chat. Types come from the OpenAPI document.
import type { ApiClient } from "./client";
import type { components } from "./schema";

type Schemas = components["schemas"];

export type ChatCommand = Schemas["CommandInfo"];
export type ChatMessageAction = Schemas["MessageActionInfo"];
export type ChatExtensions = Schemas["ExtensionsOutputBody"];
export type RunCommandInput = Schemas["RunCommandInputBody"];
/** `text` is shown to the caller only; it is empty when the command posted something itself. */
export type RunCommandResult = Schemas["RunCommandOutputBody"];

export interface ExtensionsApi {
  listExtensions(): Promise<ChatExtensions>;
  runCommand(channelId: string, input: RunCommandInput): Promise<RunCommandResult>;
}

const base = "/api/v1/chat";

export function createExtensionsApi(client: ApiClient): ExtensionsApi {
  return {
    listExtensions: () => client.request(`${base}/extensions`),
    runCommand: (channelId, input) =>
      client.request(`${base}/channels/${encodeURIComponent(channelId)}/commands`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(input),
      }),
  };
}
