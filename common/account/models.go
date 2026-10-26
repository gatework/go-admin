package account

import "time"

type State struct {
	UserID            int    `gorm:"primaryKey;autoIncrement:false"`
	Secret            string `gorm:"size:512" json:"-"`
	PendingCredential string `gorm:"size:64"`
	PendingUntil      int64
	Enabled           bool
	LastStep          int64
	Version           int64
}

func (State) TableName() string { return "sys_account_security" }

type Challenge struct {
	Hash       string `gorm:"primaryKey;size:64"`
	Kind       string `gorm:"size:16;index"`
	Email      string `gorm:"size:128;index"`
	UserID     int
	CodeHash   string `gorm:"size:64"`
	Credential string `gorm:"size:64"`
	ExpiresAt  int64
	Attempts   int
	Used       bool
}

func (Challenge) TableName() string { return "sys_account_challenge" }

type Recovery struct {
	Hash   string `gorm:"primaryKey;size:64"`
	UserID int    `gorm:"index"`
}

func (Recovery) TableName() string { return "sys_account_recovery" }

type Limit struct {
	Key       string `gorm:"primaryKey;size:80"`
	Count     int
	ExpiresAt int64
}

func (Limit) TableName() string { return "sys_account_limit" }

type Policy struct {
	ID      int `gorm:"primaryKey;autoIncrement:false"`
	Enabled bool
}

func (Policy) TableName() string { return "sys_account_policy" }

// User is the authentication projection; no API serializes this model.
type User struct {
	SessionVersion int64 `gorm:"-"`
	UserId         int   `gorm:"primaryKey;autoIncrement"`
	Username       string
	Password       string
	NickName       string
	Email          string
	RoleId         int
	DeptId         int
	PostId         int
	Status         string
	DeletedAt      int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (User) TableName() string { return "sys_user" }

type Role struct {
	RoleId    int `gorm:"primaryKey;autoIncrement"`
	RoleKey   string
	RoleName  string
	Status    string
	DataScope string
	Admin     bool
	DeletedAt int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (Role) TableName() string { return "sys_role" }
