package service

import (
	"fmt"
	"log/slog"
	"strings"

	"gbevent/internal/constants"
	"gbevent/internal/model"
	"gbevent/internal/repository"
	"gbevent/internal/util"

	"gorm.io/gorm"
)

// GroupMemberInput 团体报名单个参加人（服务层入参，结构与 dto.GroupMemberInput 对齐）。
type GroupMemberInput struct {
	Name   string
	Phone  string
	Remark string
}

// GroupView 团体报名详情：团体头 + 每个参加人（含各自的入场凭证号）。
type GroupView struct {
	Group   *model.RegistrationGroup `json:"group"`
	Members []model.Registration     `json:"members"`
}

// RegistrationGroupService 团体报名业务逻辑。
type RegistrationGroupService struct {
	db          *gorm.DB
	groupRepo   *repository.RegistrationGroupRepository
	regRepo     *repository.RegistrationRepository
	activitySvc *ActivityService
	logger      *slog.Logger
}

// NewRegistrationGroupService 构造团体报名服务。
func NewRegistrationGroupService(db *gorm.DB, groupRepo *repository.RegistrationGroupRepository,
	regRepo *repository.RegistrationRepository, activitySvc *ActivityService, logger *slog.Logger) *RegistrationGroupService {
	return &RegistrationGroupService{db: db, groupRepo: groupRepo, regRepo: regRepo, activitySvc: activitySvc, logger: logger}
}

// Create 团体报名：校验活动开启了团体报名、人数合法、整团名额足够，
// 在同一事务内写入团体记录与每名参加人的报名行，每人各自生成入场凭证号。
func (s *RegistrationGroupService) Create(activityID, userID uint64, members []GroupMemberInput) (*GroupView, error) {
	s.logger.Info(constants.LogGroupCreateStart, "activity_id", activityID, "user_id", userID, "members", len(members))
	if err := validateMembers(members); err != nil {
		return nil, err
	}
	view := &GroupView{}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		act, err := s.activitySvc.FindByIDForUpdateTx(tx, activityID)
		if err != nil {
			return err
		}
		if !act.GroupSignupEnabled {
			return util.NewAppError(constants.CodeGroupInvalid, constants.MsgGroupSignupOnly)
		}
		size := len(members)
		if size > act.GroupMaxSize {
			return util.NewAppError(constants.CodeGroupInvalid, constants.MsgGroupExceedMaxSize)
		}
		// 整团占座：剩余名额不足整团时整团失败（活动行已锁定，并发安全）。
		if err := s.activitySvc.CheckSeatsLimitTx(tx, activityID, size); err != nil {
			return err
		}
		// 同一用户对同一活动仅允许一次有效报名（单人或团体），防止重复占座。
		if _, err := s.regRepo.FindByActivityAndUserTx(tx, activityID, userID); err == nil {
			return util.NewAppError(constants.CodeDuplicateSignup, constants.MsgDuplicateSignup)
		} else if !errNotFound(err) {
			return err
		}
		group := &model.RegistrationGroup{
			ActivityID:  activityID,
			UserID:      userID,
			MemberCount: size,
			Status:      constants.GroupStatusRegistered,
		}
		if err := s.groupRepo.CreateTx(tx, group); err != nil {
			s.logger.Error(constants.LogGroupCreateFailed, "error", err)
			return util.Wrap(err, "RegistrationGroup[activity_id=%d,user_id=%d] create failed", activityID, userID)
		}
		regs, err := s.buildMemberRegistrations(tx, activityID, userID, group.ID, members)
		if err != nil {
			return err
		}
		if err := s.regRepo.CreateBatchTx(tx, regs); err != nil {
			s.logger.Error(constants.LogGroupCreateFailed, "error", err)
			return util.Wrap(err, "RegistrationGroup[id=%d] members create failed", group.ID)
		}
		vouchers := make([]string, 0, len(regs))
		for _, r := range regs {
			vouchers = append(vouchers, r.VoucherNo)
		}
		content := fmt.Sprintf("团体报名成功，共 %d 人，凭证号：%s", len(regs), strings.Join(vouchers, "、"))
		if err := s.activitySvc.CreateSignupNotificationTx(tx, userID, constants.NotificationSignupSuccess,
			constants.MsgGroupSignupSuccess, content); err != nil {
			return err
		}
		view.Group = group
		view.Members = make([]model.Registration, 0, len(regs))
		for _, r := range regs {
			view.Members = append(view.Members, *r)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.logger.Info(constants.LogGroupCreateSuccess, "group_id", view.Group.ID,
		"activity_id", activityID, "members", view.Group.MemberCount)
	return view, nil
}

// buildMemberRegistrations 为每名参加人生成报名行与互不重复的凭证号（含批次内去重）。
func (s *RegistrationGroupService) buildMemberRegistrations(tx *gorm.DB, activityID, userID, groupID uint64,
	members []GroupMemberInput) ([]*model.Registration, error) {
	seenVouchers := make(map[string]struct{}, len(members))
	regs := make([]*model.Registration, 0, len(members))
	for _, m := range members {
		voucher, err := s.uniqueVoucher(tx, seenVouchers)
		if err != nil {
			return nil, err
		}
		seenVouchers[voucher] = struct{}{}
		regs = append(regs, &model.Registration{
			ActivityID:   activityID,
			UserID:       userID,
			GroupID:      groupID,
			Name:         strings.TrimSpace(m.Name),
			Phone:        strings.TrimSpace(m.Phone),
			Remark:       strings.TrimSpace(m.Remark),
			VoucherNo:    voucher,
			Status:       constants.RegistrationStatusRegistered,
			ReviewStatus: constants.ReviewStatusPending,
		})
	}
	return regs, nil
}

// uniqueVoucher 在事务内生成凭证号，冲突（批次内或库内唯一索引）时重试。
func (s *RegistrationGroupService) uniqueVoucher(tx *gorm.DB, batch map[string]struct{}) (string, error) {
	for attempt := 0; attempt < 5; attempt++ {
		voucher := util.GenerateVoucherNo()
		if _, ok := batch[voucher]; ok {
			continue
		}
		var n int64
		if err := tx.Model(&model.Registration{}).Where("voucher_no = ?", voucher).Count(&n).Error; err != nil {
			return "", util.Wrap(err, "voucher uniqueness check failed")
		}
		if n == 0 {
			return voucher, nil
		}
	}
	return "", util.NewAppError(constants.CodeInternalError, "generate unique voucher failed after retries")
}

// Cancel 整团取消：仅团体提交人或管理员可操作；团内任一人已签到则拒绝整团取消。
// 取消时物理删除团内成员报名行（从“我的报名/组织者名单/导出”中消失），
// 团体记录本身保留为 cancelled 作为凭证；并发取消由团体行锁串行化，名额只放回一次。
func (s *RegistrationGroupService) Cancel(id, operatorID uint64, operatorRole string) (*GroupView, error) {
	view := &GroupView{}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		group, err := s.groupRepo.FindByIDForUpdate(tx, id)
		if err != nil {
			return util.Wrap(err, "RegistrationGroup[id=%d] cancel find failed", id)
		}
		if operatorRole != constants.RoleAdmin && group.UserID != operatorID {
			return util.NewAppError(constants.CodeForbidden, "RegistrationGroup[id="+itoa(id)+"] cancel forbidden: not owner")
		}
		// 已取消的团体重复取消直接冲突（团体行已加锁，保证并发取消只有一个事务能走到删除）。
		if group.Status != constants.GroupStatusRegistered {
			return util.NewAppError(constants.CodeGroupCancelConflict, constants.MsgCancelConflict)
		}
		members, err := s.regRepo.ListByGroupIDTx(tx, id)
		if err != nil {
			return err
		}
		if len(members) == 0 {
			return util.NewAppError(constants.CodeNotFound, "RegistrationGroup[id="+itoa(id)+"] has no members")
		}
		for _, m := range members {
			if m.Status == constants.RegistrationStatusCheckedIn {
				return util.NewAppError(constants.CodeGroupCancelConflict, constants.MsgGroupCancelConflict)
			}
		}
		// 删除成员行前保留快照用于响应；删除行数必须等于成员数，避免异常情况下状态不一致。
		view.Members = members
		deleted, err := s.regRepo.DeleteByGroupIDTx(tx, id)
		if err != nil {
			return util.Wrap(err, "RegistrationGroup[id=%d] members delete failed", id)
		}
		if deleted != int64(len(members)) {
			return util.NewAppError(constants.CodeInternalError,
				"RegistrationGroup[id="+itoa(id)+"] member delete count mismatch")
		}
		group.Status = constants.GroupStatusCancelled
		if err := s.groupRepo.UpdateTx(tx, group); err != nil {
			return util.Wrap(err, "RegistrationGroup[id=%d] cancel save failed", id)
		}
		if err := s.activitySvc.CreateSignupNotificationTx(tx, group.UserID, constants.NotificationSignupSuccess,
			constants.MsgGroupCancelSuccess, fmt.Sprintf("您的团体报名（团号 %d，共 %d 人）已取消", group.ID, group.MemberCount)); err != nil {
			return err
		}
		view.Group = group
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.logger.Info(constants.LogGroupCancelSuccess, "group_id", id)
	return view, nil
}

// Get 查询团体报名详情（含全部成员凭证），仅提交人或管理员可查看。
func (s *RegistrationGroupService) Get(id, operatorID uint64, operatorRole string) (*GroupView, error) {
	group, err := s.groupRepo.FindByID(id)
	if err != nil {
		return nil, util.Wrap(err, "RegistrationGroup[id=%d] get failed", id)
	}
	if operatorRole != constants.RoleAdmin && group.UserID != operatorID {
		return nil, util.NewAppError(constants.CodeForbidden, "RegistrationGroup[id="+itoa(id)+"] get forbidden: not owner")
	}
	return s.loadView(group)
}

// ListMine 分页查询当前用户提交的团体报名，并附带各团成员。
func (s *RegistrationGroupService) ListMine(userID uint64, page, pageSize int) ([]GroupView, int64, error) {
	groups, total, err := s.groupRepo.ListByUser(userID, page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	ids := make([]uint64, 0, len(groups))
	for i := range groups {
		ids = append(ids, groups[i].ID)
	}
	memberMap, err := s.regRepo.ListByGroupIDs(ids)
	if err != nil {
		return nil, 0, err
	}
	views := make([]GroupView, 0, len(groups))
	for i := range groups {
		g := groups[i]
		// 已取消团体的成员报名行已被物理删除，此时返回空数组而非 nil（JSON 序列化为 [] 而非 null）。
		members := memberMap[g.ID]
		if members == nil {
			members = []model.Registration{}
		}
		views = append(views, GroupView{Group: &g, Members: members})
	}
	return views, total, nil
}

// loadView 加载团体头对应的全部成员。
func (s *RegistrationGroupService) loadView(group *model.RegistrationGroup) (*GroupView, error) {
	memberMap, err := s.regRepo.ListByGroupIDs([]uint64{group.ID})
	if err != nil {
		return nil, err
	}
	members := memberMap[group.ID]
	if members == nil {
		members = []model.Registration{}
	}
	return &GroupView{Group: group, Members: members}, nil
}

// validateMembers 校验参加人列表：至少 2 人，姓名/手机号必填且团内手机号不重复。
func validateMembers(members []GroupMemberInput) error {
	if len(members) < constants.GroupMinSize {
		return util.NewAppError(constants.CodeGroupInvalid, constants.MsgGroupMinSize)
	}
	phones := make(map[string]struct{}, len(members))
	for i, m := range members {
		name := strings.TrimSpace(m.Name)
		phone := strings.TrimSpace(m.Phone)
		if name == "" {
			return util.NewAppError(constants.CodeGroupInvalid, fmt.Sprintf("第 %d 位参加人姓名不能为空", i+1))
		}
		if phone == "" {
			return util.NewAppError(constants.CodeGroupInvalid, fmt.Sprintf("第 %d 位参加人手机号不能为空", i+1))
		}
		if _, ok := phones[phone]; ok {
			return util.NewAppError(constants.CodeGroupInvalid, fmt.Sprintf("手机号 %s 在团内重复填写", phone))
		}
		phones[phone] = struct{}{}
	}
	return nil
}
