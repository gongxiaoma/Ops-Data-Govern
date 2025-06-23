package es

import api "github.com/flipped-aurora/gin-vue-admin/server/api/v1"

type RouterGroup struct {
	// 定义路由
	AliEsSlowSearchRouter
}

var (
	// 调用api下接口
	aliEsSlowSearchLog = api.ApiGroupApp.ElasticSearchApiGroup.AliEsSlowSearchLogApi
)
