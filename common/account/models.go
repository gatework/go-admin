package account

import "time"

// State defines the state contract.
type State struct {
	UserID            int    `gorm:"primaryKey;autoIncrement:false"`
	Secret            string `gorm:"size:512" json:"-"`
	PendingCredential string `gorm:"size:64"`
	PendingUntil      int64
	Enabled           bool
	LastStep          int64
	Version           int64
}

// TableName returns the explicit persistence table name.
func (State) TableName() string { return "sys_account_security" }

// Challenge defines the challenge contract.
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

// TableName returns the explicit persistence table name.
func (Challenge) TableName() string { return "sys_account_challenge" }

// Recovery defines the recovery contract.
type Recovery struct {
	Hash   string `gorm:"primaryKey;size:64"`
	UserID int    `gorm:"index"`
}

// TableName returns the explicit persistence table name.
func (Recovery) TableName() string { return "sys_account_recovery" }

// Limit defines the limit contract.
type Limit struct {
	Key       string `gorm:"primaryKey;size:80"`
	Count     int
	ExpiresAt int64
}

// TableName returns the explicit persistence table name.
func (Limit) TableName() string { return "sys_account_limit" }

// Policy defines the policy contract.
type Policy struct {
	ID      int `gorm:"primaryKey;autoIncrement:false"`
	Enabled bool
}

// TableName returns the explicit persistence table name.
func (Policy) TableName() string { return "sys_account_policy" }

// User is the authentication projection; no API serializes this model.
type User struct {
	SessionVersion int64 `gorm:"-"`
	UserID         int   `gorm:"primaryKey;autoIncrement"`
	Username       string
	Password       string
	NickName       string
	Email          string
	RoleID         int
	DeptID         int
	PostID         int
	Status         string
	DeletedAt      int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// TableName returns the explicit persistence table name.
func (User) TableName() string { return "sys_user" }

// Role defines the role contract.
type Role struct {
	RoleID    int `gorm:"primaryKey;autoIncrement"`
	RoleKey   string
	RoleName  string
	Status    string
	DataScope string
	Admin     bool
	DeletedAt int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableName returns the explicit persistence table name.
func (Role) TableName() string { return "sys_role" }
