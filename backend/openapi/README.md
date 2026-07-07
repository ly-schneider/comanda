# OpenAPI specs

The API is OpenAPI-first. `openapi.yaml` is the root document; it `$ref`s
per-resource files under `paths/` and `schemas/`. `just generate-api` bundles
the tree with Redocly and runs `oapi-codegen`. The bundled output
(`bundled.yaml`) and the generated Go (`internal/generated/api/generated/`) are
gitignored — regenerated, never committed.

Every operation's `operationId` must equal the Go handler method name on
`*api.Handlers` (oapi-codegen maps it onto the generated `ServerInterface`).

## Plugin endpoints

Core plugin management (`/plugins`, `/platform/tenants/{tenantId}/plugins`)
lives in the normal core files (`paths/plugins.yaml`, `schemas/plugins.yaml`).

A plugin that contributes its own endpoints keeps them inside this pipeline —
there is no escape hatch:

- Paths in `paths/plugins/<key>.yaml`, schemas in `schemas/plugins/<key>.yaml`.
- The root `openapi.yaml` gets explicit `/plugins/<key>/…` entries `$ref`-ing
  those files (in-tree plugins ⇒ committed refs, no merge tooling).
- Prefix every `operationId` with the plugin key (e.g. `club100ListPeople`), so
  it can't collide with core or another plugin, then wire the generated
  `wrapper.Club100ListPeople` inside the plugin's own route group.
