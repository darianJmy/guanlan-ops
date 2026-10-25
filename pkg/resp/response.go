package resp

import "github.com/gin-gonic/gin"

// Response is the common API envelope. Data uses a pointer to distinguish an
// omitted field from an explicitly supplied null, empty list, or zero value.
type Response struct {
	Code    int     `json:"code"`
	Message *string `json:"message,omitempty"`
	Data    *any    `json:"data,omitempty"`
}

// JSON writes a response without changing control flow or aborting middleware.
// Omit data to omit the JSON field; pass nil to explicitly return data: null.
func JSON(c *gin.Context, status, code int, message string, data ...any) {
	body := Response{Code: code, Message: &message}
	if len(data) > 0 {
		body.Data = &data[0]
	}
	c.JSON(status, body)
}

// Success preserves the caller's HTTP status and success message.
func Success(c *gin.Context, status int, message string, data ...any) {
	JSON(c, status, CodeSuccess, message, data...)
}

// Error preserves the caller's HTTP status and business error code.
func Error(c *gin.Context, status, code int, message string, data ...any) {
	JSON(c, status, code, message, data...)
}

// SuccessData returns data for endpoints whose contract has no message field.
func SuccessData(c *gin.Context, status int, data any) {
	c.JSON(status, Response{Code: CodeSuccess, Data: &data})
}
