package utils

import (
	"context"
	"time"
)

// GetContextRemainingTime 返回context剩余时间，若无超时设置返回0和false
func GetContextRemainingTime(ctx context.Context) (time.Duration, bool) {
	deadline, ok := ctx.Deadline()
	if !ok {
		return 0, false
	}
	return time.Until(deadline), true
}
