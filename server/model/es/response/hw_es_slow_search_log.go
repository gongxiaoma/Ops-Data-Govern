package response

// HwSlowSearchLogResponse 定义华为云ShowLogBackup接口返回的结构体
type HwSlowSearchLogResponse struct {
	LogList struct {
		Content string `json:"content"`
		Date    string `json:"date"`
		Level   string `json:"level"`
	} `json:"logList"`
}
