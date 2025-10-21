package result

type HotThreadResult struct {
	EsTag        string    `json:"esTag"`
	InstanceID   string    `json:"instanceId"`
	Endpoint     string    `json:"endpoint"`
	Timestamp    int64     `json:"timestamp"`
	UsagePercent []float64 `json:"usagePercent"`
	RawResponse  string    `json:"rawResponse"`
	HotInterval  string    `json:"hotInterval"`
	HotThreads   int       `json:"hotThreads"`
	HotType      string    `json:"hotType"`
}
