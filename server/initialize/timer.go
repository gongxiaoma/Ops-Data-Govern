package initialize

import (
	"fmt"
	"github.com/flipped-aurora/gin-vue-admin/server/task"
	"github.com/flipped-aurora/gin-vue-admin/server/task/es"
	"github.com/robfig/cron/v3"

	"github.com/flipped-aurora/gin-vue-admin/server/global"
)

func Timer() {
	go func() {
		var option []cron.Option
		option = append(option, cron.WithSeconds())
		// 清理DB定时任务
		_, err := global.GVA_Timer.AddTaskByFunc("ClearDB", "@daily", func() {
			err := task.ClearTable(global.GVA_DB) // 定时任务方法定在task文件包中
			if err != nil {
				fmt.Println("timer error:", err)
			}
		}, "定时清理数据库【日志，黑名单】内容", option...)
		if err != nil {
			fmt.Println("add timer error:", err)
		}

		// 查询阿里云ES慢查询日志定时任务
		aliTaskInterval := global.GVA_CONFIG.AliyunEs.TaskInterval
		_, _err := global.GVA_Timer.AddTaskByFunc("AliEsSlowSearchLogSync", aliTaskInterval, func() {
			err := es.AliyunEsSlowLogs()
			if err != nil {
				fmt.Println("timer error:", err)
			}
		}, "定时同步阿里云ES慢查询日志", option...)
		if _err != nil {
			fmt.Println("add timer error:", err)
		}

		// 查询华为云ES慢查询日志定时任务
		hwTaskInterval := global.GVA_CONFIG.HwyunEs.TaskInterval
		_, _err2 := global.GVA_Timer.AddTaskByFunc("HwEsSlowSearchLogSync", hwTaskInterval, func() {
			err := es.HwyunEsSlowLogs()
			if err != nil {
				fmt.Println("timer error:", err)
			}
		}, "定时同步华为云ES慢查询日志", option...)
		if _err2 != nil {
			fmt.Println("add timer error:", err)
		}

		// 查询ES热点线程
		esHotThreadTaskInterval := global.GVA_CONFIG.EsHotThread.TaskInterval
		_, _err3 := global.GVA_Timer.AddTaskByFunc("EsHotThread", esHotThreadTaskInterval, func() {
			err := es.HotThreadsSync()
			if err != nil {
				fmt.Println("timer error:", err)
			}
		}, "定时查询ES热点线程", option...)
		if _err3 != nil {
			fmt.Println("add timer error:", err)
		}

		// ES慢查询巡检上报
		esSlowSearchInspectionTaskInterval := global.GVA_CONFIG.EsInspection.TaskInterval
		_, _err4 := global.GVA_Timer.AddTaskByFunc("SlowSearchInspection", esSlowSearchInspectionTaskInterval, func() {
			err := es.SlowSearchInspection()
			if err != nil {
				fmt.Println("timer error:", err)
			}
		}, "ES慢查询巡检上报", option...)
		if _err4 != nil {
			fmt.Println("add timer error:", err)
		}
	}()
}
