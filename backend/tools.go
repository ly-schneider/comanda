//go:build tools

// Pins the libraries that only generated code imports: kin-openapi and
// oapi-codegen's runtime from the generated API server, swag from the generated
// Swagger docs.
//
// Generated sources are gitignored, so `go mod tidy` run without them present —
// Dependabot does exactly this — drops these from the graph, taking their
// transitive go.sum entries with them. The next build that regenerates the code
// then cannot resolve them. These blank imports keep tidy honest whether or not
// the generated files exist.
package tools

import (
	_ "github.com/getkin/kin-openapi/openapi3"
	_ "github.com/oapi-codegen/runtime"
	_ "github.com/oapi-codegen/runtime/types"
	_ "github.com/swaggo/swag"
)
