package es

// EsHotThread 定义ES索引结构体
type EsHotThread struct {
	//global.GVA_MODEL，不是MySQL结构，所以不需要依赖基础模型
	Timestamp    int64     `json:"@timestamp"`   // 时间戳
	EsTag        string    `json:"esTag"`        // ES集群标签
	InstanceId   string    `json:"instanceId"`   // ES集群ID
	UsagePercent []float64 `json:"usagePercent"` // 线程在该类型（cpu/wait/block）上的使用比例
	RawText      string    `json:"rawText"`      // 存放完整文本
	HotInterval  string    `json:"hotInterval"`  // 热点线程参数：执行间隔
	HotThreads   int       `json:"hotThreads"`   // 热点线程参数：线程
	HotType      string    `json:"hotType"`      // 热点线程参数：类型
}

func (EsHotThread) TableName() string {
	return "es_hot_thread" // 虚拟表名，实际使用ES存储
}
