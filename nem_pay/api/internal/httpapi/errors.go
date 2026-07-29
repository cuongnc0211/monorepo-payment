package httpapi

import "github.com/gin-gonic/gin"

// apiError is the single error envelope for the whole /v1 surface:
//
//	{ "error": { "type", "code", "message", "param" } }
//
// Every handler renders errors through respondError so a validation error, an auth error and
// a 404 all look identical to a client — one shape to parse, everywhere (see nem_pay/CLAUDE.md
// "API conventions").
type apiError struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Param   string `json:"param,omitempty"`
}

// Error type constants — the coarse machine-readable class of failure.
const (
	errTypeInvalidRequest = "invalid_request_error"
	errTypeAuth           = "authentication_error"
	errTypeIdempotency    = "idempotency_error"
	errTypeNotFound       = "not_found_error"
	errTypeAPI            = "api_error"
)

// respondError writes the envelope and aborts the gin chain so no later handler runs.
func respondError(c *gin.Context, status int, typ, code, message, param string) {
	c.AbortWithStatusJSON(status, gin.H{"error": apiError{
		Type: typ, Code: code, Message: message, Param: param,
	}})
}
