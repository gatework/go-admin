package models

// SysDept defines the sysdept contract.
//
//nolint:embeddedstructfieldcheck // Frozen migration field order defines historical positional SQL seed columns.
type SysDept struct {
	DeptID   int    `json:"deptId" gorm:"primaryKey;autoIncrement;"`
	ParentID int    `json:"parentId" gorm:""`
	DeptPath string `json:"deptPath" gorm:"size:255;"`
	DeptName string `json:"deptName"  gorm:"size:128;"`
	Sort     int    `json:"sort" gorm:"size:4;"`
	Leader   string `json:"leader" gorm:"size:128;"`
	Phone    string `json:"phone" gorm:"size:11;"`
	Email    string `json:"email" gorm:"size:64;"`
	Status   int    `json:"status" gorm:"size:4;"`
	ControlBy
	ModelTime
}

// TableName returns the explicit persistence table name.
func (SysDept) TableName() string {
	return "sys_dept"
}
