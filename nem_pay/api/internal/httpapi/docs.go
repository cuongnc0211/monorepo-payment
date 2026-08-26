package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	api "github.com/nempay/api"
)

// Interactive API docs (Scalar), served by the gateway itself so `docker compose up` yields
// browsable docs at /docs, versioned with the code. The spec is served locally from the embedded
// openapi.yaml; only the Scalar renderer loads from a CDN at view time.
const scalarDocsHTML = `<!doctype html>
<html>
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>NemPay API reference</title>
  </head>
  <body>
    <script id="api-reference" data-url="/openapi.yaml"></script>
    <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
  </body>
</html>`

// openapiSpec serves the embedded OpenAPI document (the single source of truth for the /v1 contract).
func openapiSpec(c *gin.Context) {
	c.Data(http.StatusOK, "application/yaml; charset=utf-8", api.OpenAPISpec)
}

// docsPage serves the Scalar reference UI, which reads /openapi.yaml.
func docsPage(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(scalarDocsHTML))
}
