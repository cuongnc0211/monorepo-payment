// Package api holds module-root, build-time embedded assets. Currently just the OpenAPI spec, so
// the /docs endpoint can serve it straight from the binary — the runtime image ships only the
// binary (no openapi.yaml on disk). Kept at the module root so go:embed can reach the spec that
// also feeds the portal's generated client; there is a single source of truth for the contract.
package api

import _ "embed"

//go:embed openapi.yaml
var OpenAPISpec []byte
