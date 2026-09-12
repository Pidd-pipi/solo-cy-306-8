package dto

import "time"

// ActivityCreateRequest 创建活动请求。
type ActivityCreateRequest struct {
	Title          string    `json:"title" binding:"required,max=200"`
	Description    string    `json:"description"`
	CoverImage     string    `json:"cover_image" binding:"max=255"`
	ActivityType   string    `json:"activity_type" binding:"required,oneof=lecture training party competition"`
	StartTime      time.Time `json:"start_time" binding:"required"`
	EndTime        time.Time `json:"end_time" binding:"required"`
	Location       string    `json:"location" binding:"max=255"`
	Capacity       int       `json:"capacity" binding:"min=0"`
	SignupDeadline time.Time `json:"signup_deadline" binding:"required"`
	Status         string    `json:"status" binding:"omitempty,oneof=draft published ended"`
	// GroupSignupEnabled 是否开启团体报名；GroupMaxSize 单团人数上限（开启团体报名时必填且 >= 2）。
	GroupSignupEnabled bool `json:"group_signup_enabled"`
	GroupMaxSize       int  `json:"group_max_size" binding:"min=0,max=100"`
}

// ActivityUpdateRequest 更新活动请求。
type ActivityUpdateRequest struct {
	Title        string `json:"title" binding:"max=200"`
	Description  string `json:"description"`
	CoverImage   string `json:"cover_image" binding:"max=255"`
	ActivityType string `json:"activity_type" binding:"omitempty,oneof=lecture training party competition"`
	Location     string `json:"location" binding:"max=255"`
	Capacity     *int   `json:"capacity" binding:"min=0"`
	// GroupSignupEnabled / GroupMaxSize 使用指针以便区分“未提交”与“显式置为 false/0”。
	GroupSignupEnabled *bool `json:"group_signup_enabled"`
	GroupMaxSize       *int  `json:"group_max_size" binding:"omitempty,min=0,max=100"`
}
