package es

import (
	"github.com/flipped-aurora/gin-vue-admin/server/global"
	"github.com/flipped-aurora/gin-vue-admin/server/model/common/response"
	esReq "github.com/flipped-aurora/gin-vue-admin/server/model/es/request"
	"github.com/flipped-aurora/gin-vue-admin/server/utils"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type AliEsSlowSearchLogApi struct {
}

func (e *AliEsSlowSearchLogApi) GetAliEsSearchSlowLogs(c *gin.Context) {
	var req esReq.GetAliEsSlowSearchList

	// 使用 Gin 框架的 ShouldBindJSON 方法将 HTTP 请求的 JSON 格式的 body 绑定到 slowLogRequest 变量。
	// _ 表示忽略绑定过程中可能返回的错误（通常不建议忽略错误，这里可能是为了简化示例）。
	err := c.ShouldBindJSON(&req)

	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}

	err = utils.Verify(req, utils.PageInfoVerify)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}

	// 调用service，传入请求结构图req（包括前端json传过来的字段和分页信息）
	list, total, err := aliEsSlowSearchLogService.GetAliEsSearchSlowLogs(req)
	if err != nil {
		global.GVA_LOG.Error("获取失败!", zap.Error(err))
		response.FailWithMessage("获取失败", c)
		return
	}
	response.OkWithDetailed(response.PageResult{
		List:     list,
		Total:    total,
		Page:     req.Page,
		PageSize: req.PageSize,
	}, "获取成功", c)
}
