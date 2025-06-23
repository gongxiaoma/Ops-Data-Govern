package utils

import (
	"context"
	"github.com/flipped-aurora/gin-vue-admin/server/global"
	"github.com/olivere/elastic/v7"
	"go.uber.org/zap"
	"time"
)

// SleepWithContext 安全休眠
// @param context 上下文
// @param delay 时间
// @return bool 返回ture或者false
func SleepWithContext(ctx context.Context, delay time.Duration) bool {
	select {
	case <-time.After(delay): // 正常休眠
		return true
	case <-ctx.Done(): // 监听父级context取消/超时
		return false
	}
}

// CalculateDelay 指数退避算法
// @param attempt 重试次数
// @param initial 初识
// @param max 最大
// @return time.Duration 延时
func CalculateDelay(attempt int, initial, max time.Duration) time.Duration {
	delay := initial * (1 << attempt) // 等价于 initial * 2^attempt
	if delay > max {
		return max
	}
	return delay
}

// LogRetryAttempt 计算延时
// @param tryCtx 上下文
// @param index 待写入索引
// @param attempt 重试次数
// @param err 错误
// @param doc 文档
func LogRetryAttempt(tryCtx context.Context, index string, attempt int, err error, doc interface{}) {
	logFields := []zap.Field{
		zap.String("待写入索引", index),
		zap.Int("下次第几次重试", attempt+1),
		zap.Error(err),
		zap.Any("文档信息", doc),
	}

	if elastic.IsConflict(err) {
		global.GVA_LOG.Warn("ES版本冲突(即将重试)", logFields...)
	} else if tryCtx.Err() != nil {
		global.GVA_LOG.Warn("ES操作超时(即将重试)", logFields...)
	} else {
		global.GVA_LOG.Warn("ES写入错误(即将重试)", logFields...)
	}
}
