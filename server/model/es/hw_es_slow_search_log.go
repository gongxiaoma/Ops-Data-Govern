package es

// HwEsSlowSearchLog 定义ES索引结构体
type HwEsSlowSearchLog struct {
	//global.GVA_MODEL，不是MySQL结构，所以不需要依赖基础模型
	EventTime  string  `json:"eventTime"`  // 日志时间
	Timestamp  int64   `json:"@timestamp"` // 时间戳
	QueryText  string  `json:"queryText"`  // 原始查询语句
	DurationMs float64 `json:"durationMs"` // 查询耗时(ms)
	IndexName  string  `json:"indexName"`  // 查询的索引
	InstanceId string  `json:"instanceId"` // ES集群ID
	NodeName   string  `json:"nodeName"`   // 节点名称
	Level      string  `json:"level"`      // 日志级别
	SearchHits int64   `json:"searchHits"` // 查询命中文档数
	SearchType string  `json:"searchType"` // 查询类型
	ShardId    int64   `json:"shardId"`    // 分片ID
	ShardTotal int64   `json:"shardTotal"` // 查询命令分片数
	TaskId     string  `json:"taskid"`     // 任务ID
}

func (HwEsSlowSearchLog) TableName() string {
	return "hw_es_slow_search_log" // 虚拟表名，实际使用ES存储
}
