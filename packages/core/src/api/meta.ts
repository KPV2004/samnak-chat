// The Meta type comes from the server's OpenAPI document (npm run gen:api).
// This file only re-exports it under a stable name.
import type { components } from "./schema";

export type Meta = components["schemas"]["MetaBody"];
