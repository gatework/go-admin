package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"gorm.io/gorm"

	"github.com/gatework/motor-core/sdk"
	"github.com/gatework/motor-core/sdk/runtime"
	"github.com/gatework/motor-core/storage"

	"github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/docs/admin"
)

type apiRouteKey struct {
	method string
	path   string
}

// APIRegistryTopic carries the host's route snapshot to SyncAPIs.
const APIRegistryTopic = "api_check_queue"

// SyncAPIs registers discovered business routes in each tenant database. The
// queue payload stores the route snapshot as JSON in Values["routes"] so memory
// and broker queues deliver the same representation.
func SyncAPIs(ctx context.Context, message storage.Message) error {
	return syncAPIs(ctx, message, admin.SwaggerInfoadmin.ReadDoc(), sdk.Runtime.Databases())
}

func syncAPIs(ctx context.Context, message storage.Message, document string, databases map[string]*gorm.DB) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	summaries, err := apiSummaries(document)
	if err != nil {
		return err
	}
	rows, err := discoveredAPIs(message, summaries)
	if err != nil {
		return err
	}
	for tenant, db := range databases {
		if db == nil {
			return fmt.Errorf("api registry: tenant %q has no database", tenant)
		}
	}
	for tenant, db := range databases {
		if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			for _, row := range rows {
				// Attrs applies only on insert: operator and application metadata
				// remains authoritative when a route was registered before.
				if err := tx.Where(&models.SysAPI{Path: row.Path, Action: row.Action}).
					Attrs(row).FirstOrCreate(&models.SysAPI{}).Error; err != nil {
					return fmt.Errorf("register %s %q: %w", row.Action, row.Path, err)
				}
			}
			return nil
		}); err != nil {
			return fmt.Errorf("api registry: tenant %q: %w", tenant, err)
		}
	}
	return nil
}

func apiSummaries(document string) (map[apiRouteKey]string, error) {
	var spec struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal([]byte(document), &spec); err != nil {
		return nil, fmt.Errorf("api registry: decode swagger: %w", err)
	}
	if spec.Paths == nil {
		return nil, errors.New("api registry: swagger has no paths")
	}
	summaries := make(map[apiRouteKey]string, len(spec.Paths))
	for path, operations := range spec.Paths {
		for method, raw := range operations {
			method = strings.ToUpper(method)
			if !apiMethod(method) {
				continue
			}
			var operation struct {
				Summary string `json:"summary"`
			}
			if err := json.Unmarshal(raw, &operation); err != nil {
				return nil, fmt.Errorf("api registry: decode swagger %s %q: %w", method, path, err)
			}
			summaries[apiRouteKey{method: method, path: path}] = operation.Summary
		}
	}
	return summaries, nil
}

func discoveredAPIs(message storage.Message, summaries map[apiRouteKey]string) ([]models.SysAPI, error) {
	payload, ok := message.Values["routes"].(string)
	if !ok {
		return nil, errors.New("api registry: routes must be a json string")
	}
	routes := []runtime.Router{}
	if err := json.Unmarshal([]byte(payload), &routes); err != nil {
		return nil, fmt.Errorf("api registry: decode routes: %w", err)
	}
	if routes == nil {
		return nil, errors.New("api registry: routes must be a json array")
	}
	rows := make([]models.SysAPI, 0, len(routes))
	for _, route := range routes {
		if route.RelativePath == "" || route.RelativePath[0] != '/' {
			return nil, fmt.Errorf("api registry: invalid route path %q", route.RelativePath)
		}
		method := strings.ToUpper(route.HTTPMethod)
		if !apiMethod(method) {
			return nil, fmt.Errorf("api registry: invalid route method %q", route.HTTPMethod)
		}
		if !businessAPI(method, route.RelativePath) {
			continue
		}
		key := apiRouteKey{method: method, path: swaggerRoutePath(route.RelativePath)}
		rows = append(rows, models.SysAPI{
			Handle: method + " " + route.RelativePath,
			Title:  summaries[key],
			Path:   route.RelativePath,
			Action: method,
		})
	}
	return rows, nil
}

func apiMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodHead, http.MethodOptions, http.MethodConnect, http.MethodTrace:
		return true
	default:
		return false
	}
}

func businessAPI(method, path string) bool {
	if method == http.MethodHead || method == http.MethodOptions {
		return false
	}
	switch path {
	case "/api/v1/health", "/api/v1/ready", "/api/v1/metrics":
		return false
	}
	for _, prefix := range []string{"/swagger", "/static", "/form-generator", "/api/v1/sys/tables"} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return false
		}
	}
	return true
}

func swaggerRoutePath(path string) string {
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		if len(segment) > 1 && (segment[0] == ':' || segment[0] == '*') {
			segments[i] = "{" + segment[1:] + "}"
		}
	}
	return strings.Join(segments, "/")
}
