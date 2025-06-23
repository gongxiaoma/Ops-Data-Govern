package utils

import (
	"fmt"
	"github.com/elastic/go-elasticsearch/v7/esapi"
	"io"
)

// ExtractError 处理网络错误和ES响应中的HTTP错误
func ExtractError(res *esapi.Response, reqID string) error {
	// 情况1: 响应结果为空
	if res == nil {
		return fmt.Errorf("空响应 (reqID: %s)", reqID)
	}

	// 情况2: 响应结果没有错误，检查读取内容是否失败
	if !res.IsError() {
		return nil
	}

	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("读取响应体失败 (reqID: %s): %w", reqID, err)
	}

	return fmt.Errorf("ES错误[%d] (reqID: %s): %s", res.StatusCode, reqID, string(bodyBytes))
}
