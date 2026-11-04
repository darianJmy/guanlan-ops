package middleware

import (
	"io"
	"log/slog"
	"runtime/debug"
	"time"

	"guanlan/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func Access() gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		ctx := otel.GetTextMapPropagator().Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))
		ctx, span := otel.Tracer("guanlan/http").Start(ctx, c.Request.Method, trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()
		traceID := uuid.NewString()
		if sc := span.SpanContext(); sc.IsValid() {
			traceID = sc.TraceID().String()
		}
		c.Request = c.Request.WithContext(logger.WithTraceID(ctx, traceID))
		c.Set("trace_id", traceID)
		c.Header("X-Trace-ID", traceID)
		c.Header("Access-Control-Expose-Headers", "X-Trace-ID")
		c.Next()
		level := slog.LevelInfo
		if c.Writer.Status() >= 500 {
			level = slog.LevelError
		} else if c.Writer.Status() >= 400 {
			level = slog.LevelWarn
		}
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		span.SetName(c.Request.Method + " " + route)
		span.SetAttributes(attribute.String("http.request.method", c.Request.Method), attribute.String("http.route", route), attribute.Int("http.response.status_code", c.Writer.Status()))
		if c.Writer.Status() >= 500 {
			span.SetStatus(codes.Error, "HTTP server error")
		}
		// Do not log query strings, request bodies, cookies or authorization headers.
		logger.LogContext(c.Request.Context(), level, "http request",
			"method", c.Request.Method, "path", path, "status", c.Writer.Status(),
			"duration_ms", float64(time.Since(started).Microseconds())/1000, "client_ip", c.ClientIP())
	}
}

func Recovery() gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, recovered any) {
		logger.LogContext(c.Request.Context(), slog.LevelError, "http panic", "panic", recovered, "stack", string(debug.Stack()))
		c.AbortWithStatus(500)
	})
}
