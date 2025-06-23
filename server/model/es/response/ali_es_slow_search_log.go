package response

// AliSlowSearchLogResponse 定义与JSON匹配的结构体
type AliSlowSearchLogResponse struct {
	ContentCollection struct {
		Content           string `json:"content"`
		Host              string `json:"host"`
		IndexName         string `json:"index_name"`
		Level             string `json:"level"`
		SearchTimeMs      string `json:"search_time_ms"`
		SearchTotalHits   string `json:"search_total_hits"`
		SearchType        string `json:"search_type"`
		ShardID           string `json:"shard_id"`
		SlowSearchLogType string `json:"slow_search_log_type"`
		Time              string `json:"time"`
		TotalShards       string `json:"total_shards"`
	} `json:"contentCollection"`
	Host       string `json:"host"`
	InstanceID string `json:"instanceId"`
	Timestamp  int64  `json:"timestamp"`
}
