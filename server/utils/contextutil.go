package utils

// 定义自己的 key 类型，防止和其他 package 的 string key 冲突
type contextKey string

// 导出一个常量，用于保存请求 ID
const RequestIDKey contextKey = "requestID"
