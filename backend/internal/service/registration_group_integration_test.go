package service

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"gbevent/internal/constants"
	"gbevent/internal/model"
	"gbevent/internal/repository"
	"gbevent/internal/util"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// 本文件为团体报名模块的集成测试，依赖真实 MySQL/MariaDB（InnoDB 行锁才能忠实
// 验证“并发报名不超额 / 并发取消只放回一次名额”）。
//
// 运行方式：
//
//	# 1) 准备一个独立测试库（与业务库隔离，只做一次）：
//	#    CREATE DATABASE gbevent_test DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
//	#    GRANT ALL ON gbevent_test.* TO 'gbevent_user'@'%';
//	# 2) go test ./internal/service/ -run 'TestGroup' -v
//
// 可通过环境变量 GBEVENT_TEST_DSN 覆盖连接串。数据库不可用时整个测试文件会被
// 跳过（skip），不影响其余不依赖数据库的单元测试。

const defaultTestDSN = "gbevent_user:gbevent_pwd@tcp(127.0.0.1:3306)/gbevent_test?charset=utf8mb4&parseTime=True&loc=Local"

var (
	testDB       *gorm.DB
	testLogger   *slog.Logger
	groupSvc     *RegistrationGroupService
	regSvc       *RegistrationService
	checkinSvc   *CheckInRecordService
	activityRepo *repository.ActivityRepository
	regRepo      *repository.RegistrationRepository
	groupRepo    *repository.RegistrationGroupRepository
)

// allTestModels 覆盖团体报名/单人报名链路涉及的全部表。
var allTestModels = []any{
	&model.User{}, &model.Activity{}, &model.Registration{}, &model.RegistrationGroup{},
	&model.CheckInRecord{}, &model.Comment{}, &model.Favorite{},
	&model.Notification{}, &model.AuditLog{},
}

func TestMain(m *testing.M) {
	testLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

	dsn := os.Getenv("GBEVENT_TEST_DSN")
	if dsn == "" {
		dsn = defaultTestDSN
	}
	// 数据库不可用时不阻断：纯单元测试照常运行，集成用例通过 skipIfNoDB 自行跳过。
	if db, err := openTestDB(dsn); err != nil {
		fmt.Printf("SKIP 集成测试：MySQL 不可用（%v）；设置 GBEVENT_TEST_DSN 后可运行团体报名集成测试\n", err)
	} else {
		// 每次运行先删除再建表，保证完全可重复、不依赖历史数据，也不触碰业务库。
		if err := db.Migrator().DropTable(allTestModels...); err != nil {
			fmt.Println("重置测试库失败:", err)
			os.Exit(1)
		}
		if err := db.AutoMigrate(allTestModels...); err != nil {
			fmt.Println("建表失败:", err)
			os.Exit(1)
		}
		buildTestServices(db)
		testDB = db
	}

	os.Exit(m.Run())
}

func openTestDB(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(30)
	sqlDB.SetMaxIdleConns(30)
	// 真正探测一次连通性（DSN 懒连接，Open 不一定失败）。
	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return db, nil
}

// skipIfNoDB 在缺少数据库时跳过当前集成用例（纯单元测试不受影响）。
func skipIfNoDB(t *testing.T) {
	t.Helper()
	if testDB == nil {
		t.Skip("MySQL 测试库不可用，跳过集成用例")
	}
}

func buildTestServices(db *gorm.DB) {
	activityRepo = repository.NewActivityRepository(db)
	regRepo = repository.NewRegistrationRepository(db)
	groupRepo = repository.NewRegistrationGroupRepository(db)
	notifyRepo := repository.NewNotificationRepository(db)
	checkinRepo := repository.NewCheckInRecordRepository(db)

	actSvc := NewActivityService(activityRepo, regRepo, notifyRepo, checkinRepo, testLogger)
	regSvc = NewRegistrationService(db, regRepo, actSvc, notifyRepo, testLogger)
	groupSvc = NewRegistrationGroupService(db, groupRepo, regRepo, actSvc, testLogger)
	checkinSvc = NewCheckInRecordService(db, checkinRepo, regRepo, actSvc, notifyRepo, testLogger)
}

// ---------- 测试辅助 ----------

// newGroupActivity 直接写入一个“已发布、可团体报名”的活动，避免经过与本模块无关的创建校验。
func newGroupActivity(t *testing.T, capacity, groupMaxSize int) *model.Activity {
	t.Helper()
	now := time.Now()
	a := &model.Activity{
		Title:              fmt.Sprintf("团体测试活动-%d", time.Now().UnixNano()),
		ActivityType:       constants.ActivityTypeParty,
		StartTime:          now.AddDate(0, 0, 7),
		EndTime:            now.AddDate(0, 0, 7),
		Location:           "测试场地",
		Capacity:           capacity,
		SignupDeadline:     now.AddDate(0, 0, 6),
		Status:             constants.ActivityStatusPublished,
		OrganizerID:        2,
		GroupSignupEnabled: true,
		GroupMaxSize:       groupMaxSize,
	}
	if err := testDB.Create(a).Error; err != nil {
		t.Fatalf("创建活动失败: %v", err)
	}
	return a
}

// newNormalActivity 写入一个仅支持单人报名的活动。
func newNormalActivity(t *testing.T, capacity int) *model.Activity {
	t.Helper()
	a := newGroupActivity(t, capacity, 0)
	a.GroupSignupEnabled = false
	a.GroupMaxSize = 0
	if err := testDB.Save(a).Error; err != nil {
		t.Fatalf("更新活动失败: %v", err)
	}
	return a
}

// makeMembers 生成 n 名参加人，手机号从 basePhone 递增，保证团内不重复。
func makeMembers(n int, basePhone uint64) []GroupMemberInput {
	out := make([]GroupMemberInput, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, GroupMemberInput{
			Name:   fmt.Sprintf("队员%02d", i+1),
			Phone:  fmt.Sprintf("%d", basePhone+uint64(i)),
			Remark: "",
		})
	}
	return out
}

// seedRegisteredRows 直接写入 cnt 条“已报名”记录（占用名额），用于制造剩余名额场景。
func seedRegisteredRows(t *testing.T, activityID uint64, cnt int, baseUser uint64, basePhone uint64) {
	t.Helper()
	for i := 0; i < cnt; i++ {
		r := &model.Registration{
			ActivityID:   activityID,
			UserID:       baseUser + uint64(i),
			GroupID:      0,
			Name:         fmt.Sprintf("占位%02d", i+1),
			Phone:        fmt.Sprintf("%d", basePhone+uint64(i)),
			VoucherNo:    fmt.Sprintf("GBT%s%07d", time.Now().Format("20060102"), basePhone+uint64(i)),
			Status:       constants.RegistrationStatusRegistered,
			ReviewStatus: constants.ReviewStatusApproved,
		}
		if err := testDB.Create(r).Error; err != nil {
			t.Fatalf("写入占座报名失败: %v", err)
		}
	}
}

// countRegistered 统计活动当前未取消报名人数（名额口径）。
func countRegistered(t *testing.T, activityID uint64) int64 {
	t.Helper()
	n, err := activityRepo.CountRegistered(activityID)
	if err != nil {
		t.Fatalf("统计报名人数失败: %v", err)
	}
	return n
}

func appErrorCode(t *testing.T, err error) int {
	t.Helper()
	var ae *util.AppError
	if errors.As(err, &ae) {
		return ae.Code
	}
	if err != nil {
		t.Fatalf("期望 AppError，实际为 %T: %v", err, err)
	}
	return constants.CodeOK
}

func requireErrCode(t *testing.T, err error, want int, ctx string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s：期望错误码 %d，实际成功", ctx, want)
	}
	if got := appErrorCode(t, err); got != want {
		t.Fatalf("%s：期望错误码 %d，实际 %d（%v）", ctx, want, got, err)
	}
}

// runConcurrently 用同步屏障让 fn 个任务同一时刻起跑，以最大化竞争窗口。
func runConcurrently(n int, fn func(i int)) {
	var wg sync.WaitGroup
	start := make(chan struct{})
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			<-start
			fn(i)
		}(i)
	}
	close(start)
	wg.Wait()
}

// ---------- 用例 ----------

// TestGroupCreateValidation 覆盖：至少两人、姓名必填、团内手机号不重复、超出单团上限、
// 在未开启团体报名的活动上提交团体报名。
func TestGroupCreateValidation(t *testing.T) {
	skipIfNoDB(t)
	t.Run("少于两人被拒", func(t *testing.T) {
		a := newGroupActivity(t, 10, 5)
		err := requireCreateErr(t, a.ID, 7001, makeMembers(1, 13900000001))
		requireErrCode(t, err, constants.CodeGroupInvalid, "1 人团体")
	})

	t.Run("两人即可报名成功", func(t *testing.T) {
		a := newGroupActivity(t, 10, 5)
		view, err := groupSvc.Create(a.ID, 7002, makeMembers(2, 13900010001))
		if err != nil {
			t.Fatalf("2 人团体应成功: %v", err)
		}
		if len(view.Members) != 2 || view.Group.MemberCount != 2 {
			t.Fatalf("团体人数不符: group=%d members=%d", view.Group.MemberCount, len(view.Members))
		}
	})

	t.Run("姓名必填", func(t *testing.T) {
		a := newGroupActivity(t, 10, 5)
		ms := makeMembers(2, 13900020001)
		ms[0].Name = "   "
		err := requireCreateErr(t, a.ID, 7003, ms)
		requireErrCode(t, err, constants.CodeGroupInvalid, "空姓名")
	})

	t.Run("团内手机号不可重复", func(t *testing.T) {
		a := newGroupActivity(t, 10, 5)
		ms := makeMembers(2, 13900030001)
		ms[1].Phone = ms[0].Phone
		err := requireCreateErr(t, a.ID, 7004, ms)
		requireErrCode(t, err, constants.CodeGroupInvalid, "重复手机号")
	})

	t.Run("超出单团人数上限被拒", func(t *testing.T) {
		a := newGroupActivity(t, 10, 3) // 单团最多 3 人
		err := requireCreateErr(t, a.ID, 7005, makeMembers(4, 13900040001))
		requireErrCode(t, err, constants.CodeGroupInvalid, "4 人团超 3 人上限")
	})

	t.Run("未开启团体报名的活动拒绝团体报名", func(t *testing.T) {
		a := newNormalActivity(t, 10)
		err := requireCreateErr(t, a.ID, 7006, makeMembers(2, 13900050001))
		requireErrCode(t, err, constants.CodeGroupInvalid, "普通活动团体报名")
	})

	t.Run("每名成员各生成独立凭证号", func(t *testing.T) {
		a := newGroupActivity(t, 10, 5)
		view, err := groupSvc.Create(a.ID, 7007, makeMembers(3, 13900060001))
		if err != nil {
			t.Fatalf("3 人团体应成功: %v", err)
		}
		seen := map[string]bool{}
		for _, m := range view.Members {
			if !util.ValidateVoucherFormat(m.VoucherNo) {
				t.Fatalf("凭证号格式非法: %q", m.VoucherNo)
			}
			if seen[m.VoucherNo] {
				t.Fatalf("凭证号重复: %s", m.VoucherNo)
			}
			seen[m.VoucherNo] = true
		}
		if len(seen) != 3 {
			t.Fatalf("应有 3 个不同凭证号，实际 %d", len(seen))
		}
	})
}

func requireCreateErr(t *testing.T, activityID, userID uint64, ms []GroupMemberInput) error {
	t.Helper()
	_, err := groupSvc.Create(activityID, userID, ms)
	if err == nil {
		t.Fatalf("期望团体报名被拒绝，但成功了")
	}
	return err
}

// TestGroupWholeQuotaReject 剩余名额只够一部分时整团拒绝，不能只报一部分。
func TestGroupWholeQuotaReject(t *testing.T) {
	skipIfNoDB(t)
	a := newGroupActivity(t, 5, 5)                    // 总名额 5
	seedRegisteredRows(t, a.ID, 3, 7100, 13900100000) // 已占 3，剩 2

	if n := countRegistered(t, a.ID); n != 3 {
		t.Fatalf("前置占座后应是 3 人，实际 %d", n)
	}

	// 3 人团需要 3 个名额，只剩 2，整团失败。
	err := requireCreateErr(t, a.ID, 7110, makeMembers(3, 13900110001))
	requireErrCode(t, err, constants.CodeGroupFull, "剩余名额不足整团")

	// 整团失败不留任何痕迹：人数仍为 3，没有新增团体记录。
	if n := countRegistered(t, a.ID); n != 3 {
		t.Fatalf("整团失败后人数应仍为 3，实际 %d", n)
	}
	var groupCnt int64
	if err := testDB.Model(&model.RegistrationGroup{}).Where("activity_id = ?", a.ID).Count(&groupCnt).Error; err != nil {
		t.Fatal(err)
	}
	if groupCnt != 0 {
		t.Fatalf("整团失败不应留下团体记录，实际 %d 条", groupCnt)
	}

	// 2 人团恰好填满（3+2=5）应成功，证明不是“一刀切禁止报名”而是整团口径。
	if _, err := groupSvc.Create(a.ID, 7111, makeMembers(2, 13900120001)); err != nil {
		t.Fatalf("2 人团恰好填满应成功: %v", err)
	}
	if n := countRegistered(t, a.ID); n != 5 {
		t.Fatalf("填满后应是 5 人，实际 %d", n)
	}

	// 已满后任何团体都应被拒。
	err = requireCreateErr(t, a.ID, 7112, makeMembers(2, 13900130001))
	requireErrCode(t, err, constants.CodeGroupFull, "满员后团体报名")
}

// TestGroupConcurrentNoOversell 多人同时报名，总人数不能超过活动名额。
func TestGroupConcurrentNoOversell(t *testing.T) {
	skipIfNoDB(t)
	a := newGroupActivity(t, 5, 4) // 容量 5；每个并发团 2 人，最多 2 个团成功（4 人）
	const n = 8
	errs := make([]error, n)
	runConcurrently(n, func(i int) {
		_, errs[i] = groupSvc.Create(a.ID, uint64(7200+i), makeMembers(2, 13900200000+uint64(i*10)))
	})

	ok, full, other := 0, 0, 0
	for _, e := range errs {
		switch {
		case e == nil:
			ok++
		case appErrorCode(t, e) == constants.CodeGroupFull:
			full++
		default:
			other++
			t.Errorf("并发报名出现非预期错误: %v", e)
		}
	}
	if other > 0 {
		t.Fatalf("存在非名额类错误，见上")
	}
	if ok != 2 {
		t.Fatalf("容量 5 / 每团 2 人，应恰好 2 个团成功，实际成功 %d（满员拒绝 %d）", ok, full)
	}
	if ok+full != n {
		t.Fatalf("成功+拒绝应等于并发数 %d，实际 %d+%d", n, ok, full)
	}
	// 关键断言：总人数绝不超过容量。
	if n := countRegistered(t, a.ID); n != 4 {
		t.Fatalf("并发后总报名人数应为 4，实际 %d（超额？）", n)
	}
	// 失败的团不应留下团体头记录。
	var groupCnt int64
	if err := testDB.Model(&model.RegistrationGroup{}).Where("activity_id = ? AND status = ?", a.ID, constants.GroupStatusRegistered).Count(&groupCnt).Error; err != nil {
		t.Fatal(err)
	}
	if groupCnt != 2 {
		t.Fatalf("应有 2 条成功团体记录，实际 %d", groupCnt)
	}
}

// TestGroupCancelRemovesMembers 整团取消后成员从我的报名、组织者名单、导出中消失，
// 团体记录本身保留为已取消。
func TestGroupCancelRemovesMembers(t *testing.T) {
	skipIfNoDB(t)
	const owner = uint64(7300)
	a := newGroupActivity(t, 10, 5)
	view, err := groupSvc.Create(a.ID, owner, makeMembers(3, 13900300001))
	if err != nil {
		t.Fatalf("报名失败: %v", err)
	}
	gid := view.Group.ID

	// 取消前三处都能看到。
	list, _, err := regSvc.ListMine(owner, 1, 50)
	if err != nil || len(list) != 3 {
		t.Fatalf("取消前我的报名应有 3 行，实际 %d（err=%v）", len(list), err)
	}
	if rows, _ := regRepo.ListByActivity(a.ID); len(rows) != 3 {
		t.Fatalf("取消前组织者名单应有 3 行，实际 %d", len(rows))
	}
	_, csvBefore, err := regSvc.ExportCSV(a.ID, a.OrganizerID, constants.RoleOrganizer)
	if err != nil || !containsAll(csvBefore, []string{"队员01", "13900300001"}) {
		t.Fatalf("取消前导出应包含成员: err=%v", err)
	}

	// 整团取消。
	cancelled, err := groupSvc.Cancel(gid, owner, constants.RoleUser)
	if err != nil {
		t.Fatalf("整团取消失败: %v", err)
	}
	if cancelled.Group.Status != constants.GroupStatusCancelled {
		t.Fatalf("团体状态应为 cancelled，实际 %s", cancelled.Group.Status)
	}

	// 1) 我的报名中消失。
	if mine, _, _ := regSvc.ListMine(owner, 1, 50); len(mine) != 0 {
		t.Fatalf("取消后我的报名应为 0 行，实际 %d 行: %+v", len(mine), mine)
	}
	// 2) 组织者名单中消失（含按活动全量查询）。
	if rows, _ := regRepo.ListByActivity(a.ID); len(rows) != 0 {
		t.Fatalf("取消后组织者名单应为 0 行，实际 %d 行", len(rows))
	}
	if _, total, _ := regSvc.List(1, 50, a.ID, "", ""); total != 0 {
		t.Fatalf("取消后组织者分页名单 total 应为 0，实际 %d", total)
	}
	// 3) 导出结果中消失。
	_, csvAfter, err := regSvc.ExportCSV(a.ID, a.OrganizerID, constants.RoleOrganizer)
	if err != nil {
		t.Fatalf("导出失败: %v", err)
	}
	if containsAny(csvAfter, []string{"队员01", "队员02", "队员03", "13900300001"}) {
		t.Fatalf("取消后导出不应再包含成员，实际 CSV:\n%s", csvAfter)
	}
	// 名额释放。
	if n := countRegistered(t, a.ID); n != 0 {
		t.Fatalf("取消后名额应释放为 0，实际 %d", n)
	}

	// 团体记录保留：仍可查到，状态 cancelled，人数不被改变。
	g, err := groupRepo.FindByID(gid)
	if err != nil {
		t.Fatalf("团体记录应保留: %v", err)
	}
	if g.Status != constants.GroupStatusCancelled || g.MemberCount != 3 {
		t.Fatalf("团体记录应为 cancelled/3 人，实际 %s/%d", g.Status, g.MemberCount)
	}
	// 成员行已物理删除，但返回视图为空数组（而非 nil）。
	got, err := groupSvc.Get(gid, owner, constants.RoleUser)
	if err != nil {
		t.Fatalf("查询已取消团体失败: %v", err)
	}
	if got.Members == nil || len(got.Members) != 0 {
		t.Fatalf("已取消团体 members 应为空数组，实际 %#v", got.Members)
	}
}

// TestGroupCancelPermission 取消的权限与状态保护。
func TestGroupCancelPermission(t *testing.T) {
	skipIfNoDB(t)
	t.Run("非团主不能取消", func(t *testing.T) {
		a := newGroupActivity(t, 10, 5)
		view, err := groupSvc.Create(a.ID, 7401, makeMembers(2, 13900400001))
		if err != nil {
			t.Fatal(err)
		}
		err = cancelErr(t, view.Group.ID, 7402, constants.RoleUser)
		requireErrCode(t, err, constants.CodeForbidden, "非团主取消")
		// 被拒后成员仍在。
		if rows, _ := regRepo.ListByActivity(a.ID); len(rows) != 2 {
			t.Fatalf("被拒取消不应删除成员，实际 %d 行", len(rows))
		}
	})

	t.Run("非团主不能查看，团主与管理员可以", func(t *testing.T) {
		a := newGroupActivity(t, 10, 5)
		view, _ := groupSvc.Create(a.ID, 7403, makeMembers(2, 13900410001))
		if _, err := groupSvc.Get(view.Group.ID, 7404, constants.RoleUser); err == nil ||
			appErrorCode(t, err) != constants.CodeForbidden {
			t.Fatalf("非团主查看应 403，实际 %v", err)
		}
		if _, err := groupSvc.Get(view.Group.ID, 7403, constants.RoleUser); err != nil {
			t.Fatalf("团主应能查看: %v", err)
		}
		if _, err := groupSvc.Get(view.Group.ID, 1, constants.RoleAdmin); err != nil {
			t.Fatalf("管理员应能查看: %v", err)
		}
	})

	t.Run("有成员签到后整团取消被拒", func(t *testing.T) {
		a := newGroupActivity(t, 10, 5)
		view, _ := groupSvc.Create(a.ID, 7405, makeMembers(3, 13900420001))
		// 第一名成员凭凭证号现场签到。
		voucher := view.Members[0].VoucherNo
		if _, err := checkinSvc.CheckInByVoucher(a.ID, a.OrganizerID, voucher); err != nil {
			t.Fatalf("凭证签到失败: %v", err)
		}
		err := cancelErr(t, view.Group.ID, 7405, constants.RoleUser)
		requireErrCode(t, err, constants.CodeGroupCancelConflict, "签到后整团取消")
		// 成员行保留。
		if rows, _ := regRepo.ListByActivity(a.ID); len(rows) != 3 {
			t.Fatalf("取消被拒后成员应全部保留，实际 %d 行", len(rows))
		}
	})
}

func cancelErr(t *testing.T, gid, operator uint64, role string) error {
	t.Helper()
	_, err := groupSvc.Cancel(gid, operator, role)
	if err == nil {
		t.Fatalf("期望取消被拒绝，但成功了")
	}
	return err
}

// TestGroupRepeatedCancelConflict 重复取消返回冲突，且人数不被改变。
func TestGroupRepeatedCancelConflict(t *testing.T) {
	skipIfNoDB(t)
	a := newGroupActivity(t, 10, 5)
	view, _ := groupSvc.Create(a.ID, 7500, makeMembers(3, 13900500001))
	gid := view.Group.ID

	if _, err := groupSvc.Cancel(gid, 7500, constants.RoleUser); err != nil {
		t.Fatalf("首次取消应成功: %v", err)
	}
	// 重复取消 → 冲突。
	err := cancelErr(t, gid, 7500, constants.RoleUser)
	requireErrCode(t, err, constants.CodeGroupCancelConflict, "重复取消")
	// 管理员重复取消同样冲突。
	err = cancelErr(t, gid, 1, constants.RoleAdmin)
	requireErrCode(t, err, constants.CodeGroupCancelConflict, "管理员重复取消")
	// 人数不被改变、成员不会被二次删除（本来就为 0，且不报错）。
	g, _ := groupRepo.FindByID(gid)
	if g.MemberCount != 3 || g.Status != constants.GroupStatusCancelled {
		t.Fatalf("重复取消后团体应为 cancelled/3 人，实际 %s/%d", g.Status, g.MemberCount)
	}
	if rows, _ := regRepo.ListByGroupIDs([]uint64{gid}); len(rows[gid]) != 0 {
		t.Fatalf("成员行不应被二次影响")
	}
}

// TestGroupConcurrentCancelSingleRelease 并发取消同一团只有一个成功，名额只放回一次。
func TestGroupConcurrentCancelSingleRelease(t *testing.T) {
	skipIfNoDB(t)
	// 容量 3、3 人团占满；取消应释放且仅释放 3 个名额。
	a := newGroupActivity(t, 3, 3)
	view, _ := groupSvc.Create(a.ID, 7600, makeMembers(3, 13900600001))
	gid := view.Group.ID
	if n := countRegistered(t, a.ID); n != 3 {
		t.Fatalf("前置应为满员 3 人，实际 %d", n)
	}

	// 团主与管理员同时取消同一团。
	cancelErrs := make([]error, 2)
	runConcurrently(2, func(i int) {
		if i == 0 {
			_, cancelErrs[i] = groupSvc.Cancel(gid, 7600, constants.RoleUser)
		} else {
			_, cancelErrs[i] = groupSvc.Cancel(gid, 1, constants.RoleAdmin)
		}
	})
	successes, conflicts := 0, 0
	for _, e := range cancelErrs {
		switch {
		case e == nil:
			successes++
		case appErrorCode(t, e) == constants.CodeGroupCancelConflict:
			conflicts++
		default:
			t.Fatalf("并发取消出现非预期错误: %v", e)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("并发取消应 1 成功 1 冲突，实际 %d 成功 %d 冲突", successes, conflicts)
	}

	// 成员只被删除一次：不存在重复删除，名单为 0。
	if rows, _ := regRepo.ListByActivity(a.ID); len(rows) != 0 {
		t.Fatalf("取消后名单应为 0，实际 %d", len(rows))
	}

	// 名额只放回一次：释放 3 个名额。两个 2 人团并发抢，只可能进 1 个（共 2 人），
	// 若名额被错误地“放回两次”（6）则两个团都会成功，测试随即失败。
	signupErrs := make([]error, 2)
	runConcurrently(2, func(i int) {
		_, signupErrs[i] = groupSvc.Create(a.ID, uint64(7610+i), makeMembers(2, 13900700000+uint64(i*10)))
	})
	win := 0
	for _, e := range signupErrs {
		switch {
		case e == nil:
			win++
		case appErrorCode(t, e) == constants.CodeGroupFull:
		default:
			t.Fatalf("并发报名出现非预期错误: %v", e)
		}
	}
	if win != 1 {
		t.Fatalf("释放一次名额（3）后并发两 2 人团应恰好 1 个成功，实际 %d", win)
	}
	if n := countRegistered(t, a.ID); n != 2 {
		t.Fatalf("最终总人数应为 2（证明名额仅释放一次），实际 %d", n)
	}
}

// TestSingleAndReviewFlowUnchanged 回归：单人报名、审核、取消主流程仍可用。
func TestSingleAndReviewFlowUnchanged(t *testing.T) {
	skipIfNoDB(t)
	a := newNormalActivity(t, 10)
	reg, err := regSvc.Create(a.ID, 7700, "单人甲", "13900800001", "回归")
	if err != nil {
		t.Fatalf("单人报名失败: %v", err)
	}
	if reg.GroupID != 0 || !util.ValidateVoucherFormat(reg.VoucherNo) {
		t.Fatalf("单人报名数据异常: group=%d voucher=%s", reg.GroupID, reg.VoucherNo)
	}
	reviewed, err := regSvc.Review(reg.ID, a.OrganizerID, constants.RoleOrganizer, constants.ReviewStatusApproved)
	if err != nil {
		t.Fatalf("审核失败: %v", err)
	}
	if reviewed.ReviewStatus != constants.ReviewStatusApproved {
		t.Fatalf("审核状态应为 approved，实际 %s", reviewed.ReviewStatus)
	}
	cancelled, err := regSvc.Cancel(reg.ID, 7700, constants.RoleUser)
	if err != nil {
		t.Fatalf("单人取消失败: %v", err)
	}
	if cancelled.Status != constants.RegistrationStatusCancelled {
		t.Fatalf("单人取消状态错误: %s", cancelled.Status)
	}
}

// containsAll 报告 s 是否包含全部子串。
func containsAll(s string, subs []string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

// containsAny 报告 s 是否包含任意一个子串。
func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
