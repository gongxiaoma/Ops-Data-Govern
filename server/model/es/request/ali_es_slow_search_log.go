package request

import (
	common "github.com/flipped-aurora/gin-vue-admin/server/model/common/request"
)

// GetAliEsSlowSearchList 定义前端请求结构图
type GetAliEsSlowSearchList struct {
	common.PageInfo
	StartTime      string  `json:"startTime" form:"startTime"`           // 开始时间
	EndTime        string  `json:"endTime" form:"endTime"`               // 结束时间
	QueryText      string  `json:"queryText" form:"queryText"`           // 原始查询语句
	MinDurationMs  float64 `json:"minDurationMs" form:"minDurationMs"`   // 最小查询耗时(ms)
	MaxDurationMs  float64 `json:"maxDurationMs" form:"maxDurationMs"`   // 最大查询耗时(ms)
	IndexName      string  `json:"indexName" form:"indexName"`           // 查询的索引
	NodeIp         string  `json:"nodeIP" form:"nodeIP"`                 // 客户端IP
	InstanceId     string  `json:"instanceId" form:"instanceId"`         // ES节点名称
	Level          string  `json:"level" form:"level"`                   // 日志级别
	SearchHits     float64 `json:"searchHits" form:"searchHits"`         // 最小查询命中文档数
	SearchType     string  `json:"searchType" form:"searchType"`         // 查询类型
	SlowSearchType string  `json:"slowSearchType" form:"slowSearchType"` // 慢查询类型
}
