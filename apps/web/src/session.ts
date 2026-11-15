import { createSession } from "@samnak/core";
import { createLocalStorageTokenStore } from "./lib/token-store";

export const session = createSession({
  baseUrl: "",
  store: createLocalStorageTokenStore(window.localStorage),
});
