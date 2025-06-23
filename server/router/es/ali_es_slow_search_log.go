package es

import (
	"github.com/flipped-aurora/gin-vue-admin/server/middleware"
	"github.com/gin-gonic/gin"
)

type AliEsSlowSearchRouter struct{}

func (s *AliEsSlowSearchRouter) InitAliEsSlowSearchRouter(Router *gin.RouterGroup) {
	aliEsSlowSearchRouter := Router.Group("es").Use(middleware.OperationRecord())
	{
		aliEsSlowSearchRouter.POST("getAliEsSlowSearchLogList", aliEsSlowSearchLog.GetAliEsSearchSlowLogs) // 获取路由组
	}
}
