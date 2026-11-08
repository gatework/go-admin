package models

import (
	vxlan "github.com/gatework/go-vxlan"
	"time"
)

type Fabric struct {
	ID                int       `gorm:"primaryKey" json:"id"`
	Code              string    `gorm:"size:32;uniqueIndex;not null" json:"code"`
	Name              string    `gorm:"size:128" json:"name"`
	Version           int64     `gorm:"not null;default:1" json:"version"`
	RulesVersion      int64     `gorm:"not null;default:1" json:"rulesVersion"`
	CollectionEnabled bool      `json:"collectionEnabled"`
	RuntimeMinutes    int       `gorm:"not null;default:5" json:"runtimeMinutes"`
	ConfigMinutes     int       `gorm:"not null;default:30" json:"configMinutes"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

func (Fabric) TableName() string { return "sys_network_fabric" }

type Rule struct {
	FabricID int    `json:"fabricId"`
	Pattern  string `json:"pattern"`
	Enabled  bool   `json:"enabled"`
}
type RuleSet struct {
	ID        int       `gorm:"primaryKey;autoIncrement:false" json:"-"`
	Version   int64     `json:"version"`
	Rules     []Rule    `gorm:"serializer:json;type:text" json:"rules"`
	UpdatedAt time.Time `json:"updatedAt"`
	UpdateBy  int       `json:"-"`
}

func (RuleSet) TableName() string { return "sys_network_fabric_rules" }

type Membership struct {
	DeviceID     int       `gorm:"primaryKey;autoIncrement:false" json:"deviceId"`
	FabricID     *int      `gorm:"index" json:"fabricId"`
	Mode         string    `gorm:"size:10;not null;default:auto" json:"mode"`
	Status       string    `gorm:"size:16;index" json:"status"`
	Version      int64     `gorm:"not null;default:1" json:"version"`
	RuleVersion  int64     `json:"ruleVersion"`
	NameSnapshot string    `gorm:"size:512" json:"nameSnapshot"`
	UpdatedAt    time.Time `json:"updatedAt"`
	UpdateBy     int       `json:"-"`
}

func (Membership) TableName() string { return "sys_network_fabric_member" }

type Observation struct {
	ID                int               `gorm:"primaryKey" json:"id"`
	FabricID          int               `gorm:"index" json:"fabricId"`
	DeviceID          int               `gorm:"index:ix_fabric_evidence,priority:1" json:"deviceId"`
	CollectionID      int               `gorm:"index" json:"collectionId"`
	Dimension         string            `gorm:"size:40;index:ix_fabric_evidence,priority:2" json:"dimension"`
	Status            string            `gorm:"size:20;index" json:"status"`
	Data              vxlan.Observation `gorm:"serializer:json;type:text" json:"data"`
	ObservedAt        time.Time         `gorm:"index:ix_fabric_evidence,priority:3" json:"observedAt"`
	Version           int64             `json:"version"`
	ConfigHash        string            `gorm:"size:64" json:"configHash,omitempty"`
	SourceHash        string            `gorm:"size:64" json:"-"`
	MembershipVersion int64             `json:"membershipVersion"`
	RuleVersion       int64             `json:"ruleVersion"`
}

func (Observation) TableName() string { return "sys_network_fabric_observation" }

type Assessment struct {
	ID            int       `gorm:"primaryKey" json:"id"`
	FabricID      int       `gorm:"index" json:"fabricId"`
	EvidenceKey   string    `gorm:"size:64;uniqueIndex:ux_fabric_assessment,priority:2" json:"evidenceKey"`
	ScopeKey      string    `gorm:"size:64;uniqueIndex:ux_fabric_assessment,priority:1" json:"-"`
	RulesVersion  int64     `json:"rulesVersion"`
	CompleteScope bool      `json:"completeScope"`
	State         string    `gorm:"size:20" json:"state"`
	Result        []byte    `gorm:"type:text" json:"-"`
	CreatedAt     time.Time `json:"createdAt"`
}

func (Assessment) TableName() string { return "sys_network_fabric_assessment" }
