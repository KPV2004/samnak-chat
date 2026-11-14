// Hand-written until apps/server/api/openapi.yaml exists; replace with generated types (npm run gen:api).
export interface Meta {
  name: string;
  version: string;
  modules: string[];
}
