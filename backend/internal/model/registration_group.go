package model

import "time"

// RegistrationGroup 团体报名记录。一次团体报名生成一条团体记录，
// 名额按整团（member_count 人）计算，成员行保存在 registrations 表中并通过 group_id 关联。
type RegistrationGroup struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	ActivityID  uint64    `gorm:"not null;index" json:"activity_id"`
	UserID      uint64    `gorm:"not null;index" json:"user_id"`
	MemberCount int       `gorm:"not null" json:"member_count"`
	Status      string    `gorm:"size:20;not null;default:registered;index" json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

// TableName 指定表名。
func (RegistrationGroup) TableName() string { return "registration_groups" }
