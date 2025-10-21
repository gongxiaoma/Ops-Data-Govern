package es

import (
	"context"
	"fmt"
	"github.com/flipped-aurora/gin-vue-admin/server/config"
	"github.com/flipped-aurora/gin-vue-admin/server/global"
	esModel "github.com/flipped-aurora/gin-vue-admin/server/model/es"
	"github.com/flipped-aurora/gin-vue-admin/server/model/es/result"
	"github.com/olivere/elastic/v7"
	"go.uber.org/zap"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"
)

// InitESClient 初始化 ES 客户端
// @return es客户端
// @return error 错误
func InitESClient() (*elastic.Client, error) {
	clusterNode := global.GVA_CONFIG.Es.ClusterNode
	userName := global.GVA_CONFIG.Es.UserName
	passWord := global.GVA_CONFIG.Es.PassWord

	client, err := elastic.NewClient(
		elastic.SetURL(clusterNode...),
		elastic.SetBasicAuth(userName, passWord),
		elastic.SetSniff(false),
		elastic.SetHealthcheckTimeoutStartup(10*time.Second),
		elastic.SetHealthcheckTimeout(5*time.Second),
	)
	if err != nil {
		global.GVA_LOG.Error("ES连接失败: ", zap.Error(err))
		return nil, err
	}
	return client, nil
}

// HotThreadsSync 写入ES热点线程数据
// @return error 错误
func HotThreadsSync() (_err error) {
	esClient, err := InitESClient()
	if err != nil {
		return err
	}

	// 设置ES写入索引名
	today := time.Now().Format("2006-01-02")
	indexName := fmt.Sprintf("es_hot_threads_log-%s", today)

	// 聚合所有实例
	allInstances := append(global.GVA_CONFIG.EsHotThread.Aliyun, global.GVA_CONFIG.EsHotThread.Huawei...)

	// 循环实例并调用FetchHotThreads方法获取热点线程相关结果
	for _, instance := range allInstances {
		result, err := FetchHotThreads(instance)
		if err != nil {
			global.GVA_LOG.Warn("获取热点线程失败", zap.String("instance", instance.InstanceID), zap.Error(err))
			continue
		}

		// 构建ES文档
		doc := esModel.EsHotThread{
			Timestamp:    result.Timestamp,
			EsTag:        result.EsTag,
			InstanceId:   result.InstanceID,
			UsagePercent: result.UsagePercent,
			RawText:      result.RawResponse,
			HotInterval:  result.HotInterval,
			HotThreads:   result.HotThreads,
			HotType:      result.HotType,
		}

		ctx := context.Background()
		_, err = esClient.Index().
			Index(indexName).
			BodyJson(doc).
			Do(ctx)
		if err != nil {
			global.GVA_LOG.Error("写入ES失败", zap.String("instance", instance.InstanceID), zap.Error(err))
			continue
		}
		global.GVA_LOG.Info("成功写入ES热点线程文档", zap.String("instance", instance.InstanceID))
	}
	return nil
}

// FetchHotThreads 处理单条日志
// @return result.HotThreadResult
// @return error 错误
func FetchHotThreads(instance config.EsHotThreadInstance) (*result.HotThreadResult, error) {
	// 构建带日期的索引
	timestamp := time.Now().UnixMilli()

	// 设置超时时间
	client := &http.Client{
		Timeout: time.Duration(instance.Timeout) * time.Second,
	}

	// 发起热点线程API接口请求
	params := url.Values{}
	params.Set("interval", instance.HotInterval)
	params.Set("threads", strconv.Itoa(instance.HotThreads))
	params.Set("type", instance.HotType)
	params.Set("ignore_idle_threads", "true")
	req, err := http.NewRequest("GET", instance.Endpoint+"/_nodes/hot_threads?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}

	// 设置Basic Auth（如果存在）
	if instance.Username != "" && instance.Password != "" {
		req.SetBasicAuth(instance.Username, instance.Password)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	bodyStr := string(bodyBytes)

	// 匹配CPU百分比，如 "100.1% (500.3ms out of 500ms) cpu usage by thread"
	re := regexp.MustCompile(`([\d.]+)%\s*\([\d.]+(ms|s|m)\s+out of\s+[\d.]+(ms|s|m)\)\s*(cpu|block|wait)\s+usage by thread`)
	matches := re.FindAllStringSubmatch(bodyStr, -1)

	var usagePercent []float64
	for _, match := range matches {
		if len(match) > 1 {
			var val float64
			fmt.Sscanf(match[1], "%f", &val)
			usagePercent = append(usagePercent, val)
		}
	}

	result := &result.HotThreadResult{
		EsTag:        instance.EsTag,
		InstanceID:   instance.InstanceID,
		Endpoint:     instance.Endpoint,
		Timestamp:    timestamp,
		UsagePercent: usagePercent,
		RawResponse:  bodyStr,
		HotInterval:  instance.HotInterval,
		HotThreads:   instance.HotThreads,
		HotType:      instance.HotType,
	}

	return result, nil
}
