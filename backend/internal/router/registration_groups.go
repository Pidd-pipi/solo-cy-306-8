package router

import (
	"gbevent/internal/middleware"

	"github.com/gin-gonic/gin"
)

// registerRegistrationGroupRoutes 团体报名路由。
func (r *Router) registerRegistrationGroupRoutes(g *gin.RouterGroup) {
	groups := g.Group("/registration-groups")
	groups.Use(middleware.AuthRequired(r.cfg))
	groups.POST("", r.registrationGroup.Create)
	groups.GET("/mine", r.registrationGroup.Mine)
	groups.GET("/:id", r.registrationGroup.Get)
	groups.POST("/:id/cancel", r.registrationGroup.Cancel)
}
