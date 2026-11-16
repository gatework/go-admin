package firewall

import (
	"context"
	"unicode/utf8"

	"gorm.io/gorm"

	models "github.com/gatework/motor/app/net/models/firewall"
	"github.com/gatework/motor/app/net/service/firewall/dto"
)

// Domains provides domains for this module.
func (s *Firewall) Domains(ctx context.Context) ([]models.Domain, error) {
	out := []models.Domain{}
	e := s.db(ctx).Order("id").Find(&out).Error
	return out, e
}

// SaveDomain provides save domain for this module.
func (s *Firewall) SaveDomain(ctx context.Context, id int, in dto.DomainInput) (models.Domain, error) {
	var d models.Domain
	if !validKey(in.Code) || len(in.Code) > 64 || !validTitle(in.Name) || utf8.RuneCountInString(in.Name) > 128 {
		return d, InvalidErr
	}
	if id == 0 {
		d = models.Domain{Code: in.Code, Name: in.Name, Version: 1}
		if e := s.db(ctx).Create(&d).Error; e != nil {
			return d, conflictOnDuplicate(e)
		}
		return d, nil
	}
	q := s.db(ctx).Model(&models.Domain{}).Where("id = ? AND version = ?", id, in.Version).Updates(map[string]any{"name": in.Name, "code": in.Code, "version": gorm.Expr("version + 1")})
	if q.Error != nil {
		return d, conflictOnDuplicate(q.Error)
	}
	if q.RowsAffected != 1 {
		return d, ConflictErr
	}
	return d, s.db(ctx).First(&d, id).Error
}

func (s *Firewall) domain(ctx context.Context, id int) error {
	var d models.Domain
	if id <= 0 {
		return InvalidErr
	}
	if e := s.db(ctx).First(&d, id).Error; e != nil {
		return safeError(e)
	}
	return nil
}
