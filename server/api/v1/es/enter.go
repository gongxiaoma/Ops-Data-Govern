package es

import "github.com/flipped-aurora/gin-vue-admin/server/service"

type ApiGroup struct {
	AliEsSlowSearchLogApi
}

var (
	// 这里是调用service的方法
	aliEsSlowSearchLogService = service.ServiceGroupApp.ElasticSearchServiceGroup.AliEsSlowSearchLogService
)
