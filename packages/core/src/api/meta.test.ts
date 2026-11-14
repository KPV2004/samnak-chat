import { describe, expectTypeOf, it } from "vitest";
import type { components } from "./schema";
import type { Meta } from "./meta";

describe("Meta", () => {
  it("is the MetaBody schema generated from apps/server/api/openapi.yaml", () => {
    expectTypeOf<Meta>().toEqualTypeOf<components["schemas"]["MetaBody"]>();
    expectTypeOf<Meta>().toEqualTypeOf<{ name: string; version: string; modules: string[] }>();
  });
});
