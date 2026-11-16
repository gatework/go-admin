package handler

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/gatework/motor-core/sdk/api"

	"github.com/gatework/motor/common/request"
)

// pathID reads a path parameter that names a row: a positive integer.
func pathID(c *gin.Context, key string) (int, bool) {
	id, err := strconv.Atoi(c.Param(key))
	return id, err == nil && id > 0
}

// optionalPathID is pathID for a parameter some of the handler's routes do
// not have: an absent one is 0.
func optionalPathID(c *gin.Context, key string) (int, bool) {
	if c.Param(key) == "" {
		return 0, true
	}
	return pathID(c, key)
}

// fail answers err by its kind and code. A missing row answers as notFound,
// the capability's own not-found error, so its code names the capability.
func fail(a *api.API, err, notFound error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = fmt.Errorf("%w: %w", notFound, err)
	}
	request.Fail(a, err)
}

// pageOf cuts one page from rows already in memory and returns it with the
// total count. A page past the end is empty.
func pageOf[T any](rows []T, pageIndex, pageSize int) ([]T, int) {
	start := min(max(pageIndex-1, 0)*pageSize, len(rows))
	end := min(start+pageSize, len(rows))
	return rows[start:end], len(rows)
}
