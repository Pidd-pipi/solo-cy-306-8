package constants

// RegistrationStatus 报名状态枚举。
const (
	RegistrationStatusRegistered = "registered"
	RegistrationStatusCancelled  = "cancelled"
	RegistrationStatusCheckedIn  = "checked_in"
)

// RegistrationStatusValues 全部报名状态值。
var RegistrationStatusValues = []string{RegistrationStatusRegistered, RegistrationStatusCancelled, RegistrationStatusCheckedIn}

// ReviewStatus 报名审核状态枚举。
const (
	ReviewStatusPending  = "pending"
	ReviewStatusApproved = "approved"
	ReviewStatusRejected = "rejected"
)

// ReviewStatusValues 全部审核状态值。
var ReviewStatusValues = []string{ReviewStatusPending, ReviewStatusApproved, ReviewStatusRejected}

// CheckInMethod 签到方式枚举。
const (
	CheckInMethodVoucher = "voucher"
	CheckInMethodScan    = "scan"
)

// IsValidRegistrationStatus 校验报名状态。
func IsValidRegistrationStatus(s string) bool {
	for _, v := range RegistrationStatusValues {
		if v == s {
			return true
		}
	}
	return false
}

// GroupStatus 团体报名状态枚举（与团内成员报名状态同步流转）。
const (
	GroupStatusRegistered = "registered"
	GroupStatusCancelled  = "cancelled"
)

// GroupMinSize 团体报名最少人数；GroupMaxSizeLimit 单团人数上限的取值边界。
const (
	GroupMinSize      = 2
	GroupMaxSizeLimit = 100
)

// IsValidGroupStatus 校验团体报名状态。
func IsValidGroupStatus(s string) bool {
	return s == GroupStatusRegistered || s == GroupStatusCancelled
}

// IsValidReviewStatus 校验审核状态。
func IsValidReviewStatus(s string) bool {
	for _, v := range ReviewStatusValues {
		if v == s {
			return true
		}
	}
	return false
}
