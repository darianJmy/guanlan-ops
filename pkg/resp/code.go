// Package resp defines the application's JSON response contract.
package resp

// Business codes are independent of HTTP status codes. Keep their values stable
// because API clients use them to determine whether an operation succeeded.
const (
	CodeSuccess             = 0
	CodeFailure             = 1
	CodeBadRequest          = 400
	CodeUnauthorized        = 401
	CodeForbidden           = 403
	CodeNotFound            = 404
	CodeConflict            = 409
	CodeTooManyRequests     = 429
	CodeInternalServerError = 500
	CodeBadGateway          = 502
)
