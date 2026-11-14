import { createApiClient } from "@samnak/core";

export const api = createApiClient({
  baseUrl: "",
  getToken: () => null,
});
