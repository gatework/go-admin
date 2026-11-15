package wireless

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"

	"github.com/gatework/motor-core/sdk/contract/actions"
	models "github.com/gatework/motor/app/net/models/wireless"
	dto "github.com/gatework/motor/app/net/service/dto/wireless"
)

// ImportError describes import error for wireless operations.
type ImportError struct {
	Row     int    `json:"row"`
	Message string `json:"message"`
}

// ImportPreview describes import preview for wireless operations.
type ImportPreview struct {
	Digest string        `json:"digest"`
	Rows   int           `json:"rows"`
	Errors []ImportError `json:"errors"`
}
type importRow struct {
	id         int
	management dto.ManagementReq
}

// PreviewImport evaluates import without applying device changes.
func (s *Wireless) PreviewImport(ctx context.Context, content string, p *actions.DataPermission) (ImportPreview, error) {
	result, _, err := s.parseImport(ctx, content, p)
	return result, err
}

// CommitImport persists the validated import operation.
func (s *Wireless) CommitImport(ctx context.Context, req dto.ImportReq, p *actions.DataPermission) error {
	if req.Digest == "" || req.Digest != hash(req.Content) {
		return ConflictErr
	}
	return s.db(ctx).Transaction(func(tx *gorm.DB) error {
		scoped := *s
		scoped.DB = tx
		preview, rows, err := scoped.parseImport(ctx, req.Content, p)
		if err != nil {
			return err
		}
		if len(preview.Errors) > 0 {
			return ConflictErr
		}
		for _, row := range rows {
			m := row.management
			q := tx.Model(&models.Resource{}).Where("id = ? AND version = ?", row.id, m.Version).Updates(map[string]any{"local_alias": m.LocalAlias, "installation_location": m.InstallationLocation, "owner": m.Owner, "notes": m.Notes, "version": gorm.Expr("version + 1")})
			if q.Error != nil {
				return q.Error
			}
			if q.RowsAffected != 1 {
				return fmt.Errorf("%w: row changed", ConflictErr)
			}
		}
		return s.audit(tx, 0, 0, "management_import", int64(len(rows)))
	})
}
func (s *Wireless) parseImport(ctx context.Context, content string, p *actions.DataPermission) (ImportPreview, []importRow, error) {
	preview := ImportPreview{Digest: hash(content), Errors: []ImportError{}}
	if len(content) > 10*1024*1024 || !utf8.ValidString(content) {
		return preview, nil, InvalidErr
	}
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(content, "\ufeff")))
	header, err := reader.Read()
	if err != nil {
		return preview, nil, InvalidErr
	}
	names := []string{"id", "version", "localAlias", "installationLocation", "owner", "notes"}
	if len(header) != len(names) {
		return preview, nil, InvalidErr
	}
	for i := range names {
		if header[i] != names[i] {
			return preview, nil, InvalidErr
		}
	}
	rows := []importRow{}
	seen := map[int]bool{}
	for line := 2; ; line++ {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return preview, nil, InvalidErr
		}
		if line > 10001 {
			return preview, nil, InvalidErr
		}
		preview.Rows++
		id, err1 := strconv.Atoi(record[0])
		version, err2 := strconv.ParseInt(record[1], 10, 64)
		m := dto.ManagementReq{Version: version, LocalAlias: record[2], InstallationLocation: record[3], Owner: record[4], Notes: record[5]}
		bad := err1 != nil || err2 != nil || id <= 0 || seen[id] || !validManagement(m)
		seen[id] = true
		var row models.Resource
		if !bad {
			err = s.db(ctx).Where("id = ? AND kind = ?", id, "access_points").First(&row).Error
			bad = err != nil
		}
		if !bad {
			_, err = s.controller(ctx, row.ControllerID, p)
			bad = err != nil || row.Version != version
		}
		if bad {
			preview.Errors = append(preview.Errors, ImportError{Row: line, Message: "invalid, duplicate, inaccessible or changed AP"})
			continue
		}
		rows = append(rows, importRow{id: id, management: m})
	}
	return preview, rows, nil
}
