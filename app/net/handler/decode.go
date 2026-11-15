package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

// decodeStrictJSON decodes the request body into v and reports whether it was
// exactly one JSON value of v's shape within limit bytes: unknown fields,
// trailing data and oversized bodies are refused. Every network handler reads
// its body through here; the limit is the caller's, sized to what the
// operation legitimately carries.
func decodeStrictJSON(c *gin.Context, v any, limit int64) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(v) != nil {
		return false
	}
	return errors.Is(decoder.Decode(new(any)), io.EOF)
}
