package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"gbevent/internal/constants"
	"gbevent/internal/dto"
	"gbevent/internal/middleware"
	"gbevent/internal/service"
	"gbevent/internal/util"

	"github.com/gin-gonic/gin"
)

// RegistrationGroupHandler 团体报名 HTTP 处理器。
type RegistrationGroupHandler struct {
	svc    *service.RegistrationGroupService
	logger *slog.Logger
}

// NewRegistrationGroupHandler 构造团体报名处理器。
func NewRegistrationGroupHandler(svc *service.RegistrationGroupService, logger *slog.Logger) *RegistrationGroupHandler {
	return &RegistrationGroupHandler{svc: svc, logger: logger}
}

// Create 团体报名。
func (h *RegistrationGroupHandler) Create(c *gin.Context) {
	var req dto.GroupSignupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, constants.CodeBadRequest, "Group registration create: "+err.Error())
		return
	}
	members := make([]service.GroupMemberInput, 0, len(req.Members))
	for _, m := range req.Members {
		members = append(members, service.GroupMemberInput{Name: m.Name, Phone: m.Phone, Remark: m.Remark})
	}
	view, err := h.svc.Create(req.ActivityID, middleware.GetUserID(c), members)
	if err != nil {
		h.wrapError(c, err, "Group registration create failed")
		return
	}
	OKWithMessage(c, constants.MsgGroupSignupSuccess, view)
}

// Mine 我的团体报名。
func (h *RegistrationGroupHandler) Mine(c *gin.Context) {
	var q dto.PageQuery
	_ = c.ShouldBindQuery(&q)
	q.Normalize()
	views, total, err := h.svc.ListMine(middleware.GetUserID(c), q.Page, q.PageSize)
	if err != nil {
		h.wrapError(c, err, "Group registration mine failed")
		return
	}
	OK(c, pageResponse(views, total, q.Page, q.PageSize))
}

// Get 团体报名详情。
func (h *RegistrationGroupHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		Fail(c, http.StatusBadRequest, constants.CodeBadRequest, "Group registration[id] get: invalid id")
		return
	}
	view, err := h.svc.Get(id, middleware.GetUserID(c), middleware.GetUserRole(c))
	if err != nil {
		h.wrapError(c, err, "Group registration get failed")
		return
	}
	OK(c, view)
}

// Cancel 整团取消。
func (h *RegistrationGroupHandler) Cancel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		Fail(c, http.StatusBadRequest, constants.CodeBadRequest, "Group registration[id] cancel: invalid id")
		return
	}
	view, err := h.svc.Cancel(id, middleware.GetUserID(c), middleware.GetUserRole(c))
	if err != nil {
		h.wrapError(c, err, "Group registration cancel failed")
		return
	}
	OKWithMessage(c, constants.MsgGroupCancelSuccess, view)
}

func (h *RegistrationGroupHandler) wrapError(c *gin.Context, err error, ctx string) {
	var appErr *util.AppError
	if errors.As(err, &appErr) {
		c.Set("audit_detail", appErr.Message)
		h.logger.Warn("group registration handler error", "context", ctx, "error", appErr.Error())
		Fail(c, appErrorStatus(appErr.Code), appErr.Code, appErr.Message)
		return
	}
	h.logger.Error("group registration handler error", "context", ctx, "error", err.Error())
	Fail(c, http.StatusInternalServerError, constants.CodeInternalError, constants.MsgInternalError)
}
