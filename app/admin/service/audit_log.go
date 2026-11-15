package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gatework/motor-core/sdk"
	"github.com/gatework/motor-core/storage"

	"github.com/gatework/motor/app/admin/models"
)

// maxStoredResult bounds the response excerpt kept in sys_opera_log, in
// characters: cutting at a byte offset could split a character, which
// PostgreSQL and strict-mode MySQL refuse to store.
const maxStoredResult = 100

// StoreLoginLog is the queue consumer that writes one login record to the
// tenant database the message names.
func StoreLoginLog(ctx context.Context, message storage.Message) error {
	return storeAuditRecord(ctx, message, func(*models.SysLoginLog) {})
}

// StoreOperationLog is the queue consumer that writes one operation record,
// keeping the first maxStoredResult characters of its response.
func StoreOperationLog(ctx context.Context, message storage.Message) error {
	return storeAuditRecord(ctx, message, func(record *models.SysOperaLog) {
		if r := []rune(record.JSONResult); len(r) > maxStoredResult {
			record.JSONResult = string(r[:maxStoredResult])
		}
	})
}

// storeAuditRecord decodes the message payload into T, lets prepare adjust it
// and inserts it. Errors are returned, not logged: the queue runner reports
// them once.
func storeAuditRecord[T any](ctx context.Context, message storage.Message, prepare func(*T)) error {
	tenant, ok := message.Values[storage.PrefixKey].(string)
	if !ok {
		return errors.New("audit log message has no tenant")
	}
	db := sdk.Runtime.DB(tenant)
	if db == nil {
		return fmt.Errorf("audit log: tenant %q has no database", tenant)
	}
	payload, ok := message.Values["payload"].(string)
	if !ok {
		return errors.New("audit log message has no payload")
	}
	var record T
	if err := json.Unmarshal([]byte(payload), &record); err != nil {
		return fmt.Errorf("audit log payload: %w", err)
	}
	prepare(&record)
	if err := db.WithContext(ctx).Create(&record).Error; err != nil {
		return fmt.Errorf("storing audit record for tenant %q: %w", tenant, err)
	}
	return nil
}
