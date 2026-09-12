package dto

// SignupRequest 报名请求。
type SignupRequest struct {
	ActivityID uint64 `json:"activity_id" binding:"required"`
	Name       string `json:"name" binding:"required,max=50"`
	Phone      string `json:"phone" binding:"required,max=20"`
	Remark     string `json:"remark" binding:"max=255"`
}

// ReviewRequest 审核请求。
type ReviewRequest struct {
	ReviewStatus string `json:"review_status" binding:"required,oneof=approved rejected"`
}

// CheckInRequest 签到请求：凭证号与扫码内容二选一。
type CheckInRequest struct {
	Voucher   string `json:"voucher"`
	QRContent string `json:"qr_content"`
}

// GroupMemberInput 团体报名中的单个参加人。
type GroupMemberInput struct {
	Name   string `json:"name" binding:"required,max=50"`
	Phone  string `json:"phone" binding:"required,max=20"`
	Remark string `json:"remark" binding:"max=255"`
}

// GroupSignupRequest 团体报名请求：一次填写多名参加人（至少 2 人），整团占座、整团成败。
type GroupSignupRequest struct {
	ActivityID uint64             `json:"activity_id" binding:"required"`
	Members    []GroupMemberInput `json:"members" binding:"required,min=2,max=100,dive"`
}
