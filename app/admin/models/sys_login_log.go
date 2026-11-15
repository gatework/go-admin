package models

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	log "github.com/gatework/motor-core/logger"
	"github.com/gatework/motor-core/sdk"
	"github.com/gatework/motor-core/storage"

	"github.com/gatework/motor/common/models"
)

// SysLoginLog describes persisted administration data.
type SysLoginLog struct {
	models.Model
	models.ControlBy
	Username      string    `json:"username" gorm:"size:128;comment:用户名"`
	Status        string    `json:"status" gorm:"size:4;comment:状态"`
	Ipaddr        string    `json:"ipaddr" gorm:"size:255;comment:ip地址"`
	LoginLocation string    `json:"loginLocation" gorm:"size:255;comment:归属地"`
	Browser       string    `json:"browser" gorm:"size:255;comment:浏览器"`
	Os            string    `json:"os" gorm:"size:255;comment:系统"`
	Platform      string    `json:"platform" gorm:"size:255;comment:固件"`
	LoginTime     time.Time `json:"loginTime" gorm:"comment:登录时间"`
	Remark        string    `json:"remark" gorm:"size:255;comment:备注"`
	Msg           string    `json:"msg" gorm:"size:255;comment:信息"`
	CreatedAt     time.Time `json:"createdAt" gorm:"comment:创建时间"`
	UpdatedAt     time.Time `json:"updatedAt" gorm:"comment:最后更新时间"`
}

// TableName returns the explicit persistence table name.
func (*SysLoginLog) TableName() string {
	return "sys_login_log"
}

// Generate copies request fields into a new request or supplied model.
func (e *SysLoginLog) Generate() models.ActiveRecord {
	o := *e
	return &o
}

// GetID returns the selected resource identity.
func (e *SysLoginLog) GetID() any {
	return e.ID
}

// SaveLoginLog 从队列中获取登录日志
func SaveLoginLog(ctx context.Context, message storage.Message) (err error) {
	//准备db
	tenant, ok := message.Values[storage.PrefixKey].(string)
	if !ok {
		return errors.New("log message has no tenant")
	}
	db := sdk.Runtime.DB(tenant)
	if db == nil {
		err = errors.New("db not exist")
		log.NewHelper(log.Default()).Errorf("host[%s]'s %s", tenant, err.Error())
		return err
	}
	payload, ok := message.Values["payload"].(string)
	if !ok {
		return errors.New("log message has no payload")
	}
	var l SysLoginLog
	err = json.Unmarshal([]byte(payload), &l)
	if err != nil {
		log.NewHelper(log.Default()).Errorf("json Unmarshal error, %s", err.Error())
		return err
	}
	err = db.WithContext(ctx).Create(&l).Error
	if err != nil {
		log.NewHelper(log.Default()).Errorf("db create error, %s", err.Error())
		return err
	}
	return nil
}
