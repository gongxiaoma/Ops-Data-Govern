package es

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	aliopenapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	alies "github.com/alibabacloud-go/elasticsearch-20170613/v4/client"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/elastic/go-elasticsearch/v7"
	"github.com/flipped-aurora/gin-vue-admin/server/global"
	esModel "github.com/flipped-aurora/gin-vue-admin/server/model/es"
	esResp "github.com/flipped-aurora/gin-vue-admin/server/model/es/response"
	"github.com/flipped-aurora/gin-vue-admin/server/utils"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// AliEsClusterError 记录实例级别的错误信息
type AliEsClusterError struct {
	InstanceID string
	Error      error
}

// AliyunSdkInit 阿里云SDK初始化
// @param accessKeyId 阿里云ak
// @param accessKeySecret 阿里云sk
// @param regionId 区域
// @return aliesClient 阿里云ES客户端
// @return error 错误
func AliyunSdkInit(accessKeyId *string, accessKeySecret *string, regionId *string) (aliEsClient *alies.Client, _err error) {

	// &号表示创建了一个aliopenapi.Config类型的零值实例，并获取了这个实例的内存地址。这个地址被赋值给了config变量。因此config是一个指向aliopenapi.Config类型值的指针。
	config := &aliopenapi.Config{}
	config.AccessKeyId = accessKeyId
	config.AccessKeySecret = accessKeySecret
	config.RegionId = regionId

	// _result是一个指向alies.Client类型的指针
	ailEsClient, _err := alies.NewClient(config)
	return ailEsClient, _err
}

// AliEsClientInit 阿里云ES客户端初始化
// @return aliesClient 阿里云ES客户端
// @return error 错误
func AliEsClientInit() (aliesClient *alies.Client, _err error) {

	// 阿里云密钥
	aliyunAccessKeyIdCipherText := global.GVA_CONFIG.AliyunEs.AliyunKey
	aliyunAccessKeySecretCipherText := global.GVA_CONFIG.AliyunEs.AliyunSecret
	regionId := global.GVA_CONFIG.AliyunEs.Region

	cipherTextsMap := map[string]string{
		"aliyunAccessKeyId":     aliyunAccessKeyIdCipherText,
		"aliyunAccessKeySecret": aliyunAccessKeySecretCipherText,
	}

	cipherKeyPlainText := global.GVA_CONFIG.AliyunEs.CipherKey
	cipherKey := []byte(cipherKeyPlainText)
	decryptedMap := make(map[string]string)
	for key, cipherText := range cipherTextsMap {
		decrypted, _err := utils.DecryptAES(cipherKey, cipherText)
		if _err != nil {
			global.GVA_LOG.Error("解密失败: ", zap.Error(_err))
			return nil, _err
		}
		decryptedMap[key] = decrypted
	}

	aliyunAccessKeyId := decryptedMap["aliyunAccessKeyId"]
	aliyunAccessKeySecret := decryptedMap["aliyunAccessKeySecret"]

	// 初始阿里云化客户端
	aliyunesClient, _err := AliyunSdkInit(&aliyunAccessKeyId, &aliyunAccessKeySecret, &regionId)
	if _err != nil {
		global.GVA_LOG.Error("初始化阿里云SDK:执行失败: ", zap.Error(_err))
		return nil, _err
	} else {
		global.GVA_LOG.Info("初始化阿里云SDK:执行完成")
	}

	return aliyunesClient, _err
}

// GetAliyunEsSlowLogs 获取阿里云ES慢请求日志
// @param client 客户端
// @return error 错误
func GetAliyunEsSlowLogs(ctx context.Context, client *alies.Client) ([]AliEsClusterError, error) {

	// 方法执行开始时间
	start := time.Now()
	reqID, _ := ctx.Value(utils.RequestIDKey).(string)

	// 获取配置
	clusterNode := global.GVA_CONFIG.Es.ClusterNode
	userName := global.GVA_CONFIG.Es.UserName
	passWord := global.GVA_CONFIG.Es.PassWord

	cfg := elasticsearch.Config{
		Addresses: clusterNode, // HTTPS地址
		Username:  userName,
		Password:  passWord,
		Transport: &http.Transport{ // 自定义HTTP传输层
			ResponseHeaderTimeout: 15 * time.Second, // 从连接建立到收到响应头的总超时
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second, // TCP连接建立超时（最重要），和ResponseHeaderTimeout设置避免卡死
				KeepAlive: 30 * time.Second, // 保持连接时长
			}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second, // 非TLS时会自动跳过
			ExpectContinueTimeout: 1 * time.Second,  // 优化HTTP 100 Continue
			MaxIdleConns:          100,              // 连接池大小
			MaxIdleConnsPerHost:   20,               // 单主机连接数限制
		},
	}

	// 创建客户端
	esClient, err := elasticsearch.NewClient(cfg)
	if err != nil {
		global.GVA_LOG.Error("ES连接失败: ",
			zap.String("reqID", reqID),
			zap.Error(err))
	}

	// 从配置获取ES实例ID列表
	clusterIds := global.GVA_CONFIG.AliyunEs.EsList
	if len(clusterIds) == 0 {
		global.GVA_LOG.Error("配置文件中未找到elasticsearch实例id列表: ",
			zap.String("reqID", reqID),
			zap.Error(err))
		return nil, nil
	}

	// 设置查询时间范围（默认查询最近1小时）
	minutesBack := global.GVA_CONFIG.AliyunEs.MinutesBack
	endTime := time.Now().UnixNano() / int64(time.Millisecond)
	startTime := endTime - minutesBack*60*1000 // 减去10分钟

	// 并发控制初始化
	maxConcurrency := global.GVA_CONFIG.AliyunEs.MaxConcurrency
	var (
		wg      sync.WaitGroup                                  // 主线程等待一组goroutine的执行完毕
		errChan = make(chan AliEsClusterError, len(clusterIds)) // 管道
		sem     = make(chan struct{}, maxConcurrency)           // sem主要用于控制并发
	)

	for _, clusterId := range clusterIds {
		currentID := clusterId // 创建局部副本

		wg.Add(1)         // 通常写在启动新的goroutine之前
		sem <- struct{}{} // 获取信号量

		go func() {
			defer func() {
				<-sem     // 释放信号量
				wg.Done() // 通常写在goroutine之中
			}()

			// 处理单个实例
			if err := AliProcessInstanceSlowLogs(ctx, client, esClient, currentID, startTime, endTime); err != nil {
				// 处理单个实例，如果有返回错误，写入到channel中
				errChan <- AliEsClusterError{
					InstanceID: currentID,
					Error:      err,
				}
			}
		}()
	}

	// 单独启动goroutine等待和关闭，否则可能会阻塞主线程下面的代码
	go func() {
		wg.Wait()      // 等待所有goroutine完成
		close(errChan) // 只有当所有工作goroutine都调用了wg.Done()之后才会执行
	}()

	// 收集错误（上面单独启动goroutine，就算阻塞如下代码依然可以执行）
	var errorList []AliEsClusterError
	for e := range errChan {
		global.GVA_LOG.Error("实例处理失败",
			zap.String("reqID", reqID),
			zap.String("集群", e.InstanceID),
			zap.Error(e.Error))
		errorList = append(errorList, e)
	}

	// 返回结果
	if len(errorList) > 0 {
		return errorList, fmt.Errorf("%d个实例处理失败", len(errorList))
	}

	// 记录方法耗时和请求ID
	defer func() {
		duration := time.Since(start)
		durationMillis := duration.Milliseconds()
		global.GVA_LOG.Info("GetAliyunEsSlowLogs方法处理耗时",
			zap.String("reqID", reqID),
			zap.Int64("耗时(毫秒)", durationMillis))
	}()
	return nil, nil
}

// AliProcessInstanceSlowLogs 处理单个实例慢日志
// @param context 时间戳(毫秒)
// @param client 阿里云ES客户端（用于查询数据）
// @param esClient 本地ES客户端（用于写入数据）
// @param instanceId 持续时间(毫秒)
// @param startTime 实例 ID
// @param endTime 实例 ID
// @return error 错误
func AliProcessInstanceSlowLogs(ctx context.Context, client *alies.Client, esClient *elasticsearch.Client,
	instanceId string, startTime, endTime int64) error {

	// 统计方法执行时间
	start := time.Now()
	reqID, _ := ctx.Value(utils.RequestIDKey).(string)
	var allLogs []interface{}

	// 记录方法耗时和请求ID
	defer func() {
		duration := time.Since(start)
		durationMillis := duration.Milliseconds()

		global.GVA_LOG.Info("AliProcessInstanceSlowLogs方法处理耗时",
			zap.String("reqID", reqID),
			zap.String("集群", instanceId),
			zap.Int64("耗时(毫秒)", durationMillis))
	}()

	// 重试配置
	const (
		maxRetries    = 2
		initialDelay  = 1 * time.Second
		maxDelay      = 10 * time.Second
		perTryTimeout = 30 * time.Second
	)

	// 初始化分页参数
	const pageSize = 50
	var (
		page      int32 = 1
		totalLogs int
	)

	// 记录开始信息
	global.GVA_LOG.Info("开始查询阿里云ES实例慢查询日志",
		zap.String("reqID", reqID),
		zap.String("集群", instanceId),
		zap.Int64("开始时间", startTime),
		zap.Int64("结束时间", endTime))

	// 分页查询
	for {
		var logs []*alies.ListSearchLogResponseBodyResult
		var lastErr error

		// 重试循环
		for attempt := 0; attempt <= maxRetries; attempt++ {
			// 用于控制重试的context单独设置超时时间，和父context没有关系
			tryCloudCtx, tryCloudCancel := context.WithTimeout(ctx, perTryTimeout)

			err := func() error {
				defer tryCloudCancel()
				// 构建请求
				request := &alies.ListSearchLogRequest{
					Query:     tea.String(""),
					BeginTime: tea.Int64(startTime),
					EndTime:   tea.Int64(endTime),
					Size:      tea.Int32(pageSize),
					Page:      tea.Int32(page),
					Type:      tea.String("SEARCHSLOW"),
				}
				runtime := &util.RuntimeOptions{
					ConnectTimeout: tea.Int(5000),  // 5秒连接超时
					ReadTimeout:    tea.Int(30000), // 30秒读取超时
				}

				// 发送请求，判断打印错误，无错误使用logs变量接收
				response, err := client.ListSearchLogWithOptions(tea.String(instanceId), request, nil, runtime)
				if err != nil {
					global.GVA_LOG.Error("查询ES慢日志失败(实例ID: %s): %v",
						zap.String("reqID", reqID),
						zap.String("集群", instanceId), zap.Error(err))
					return fmt.Errorf("查询ES慢日志失败: %w", err)
				} else {
					logs = response.Body.Result
					return nil
				}
			}()

			// 如果匿名函数执行没有错误，就跳出重试for循环
			if err == nil {
				if attempt > 0 {
					global.GVA_LOG.Warn("查询ES慢日志成功(经过重试)",
						zap.String("reqID", reqID),
						zap.Int("第几次重试", attempt),
						zap.String("集群", instanceId))
				}
				break
			}

			lastErr = err
			// 如果是最后一次重试，就跳出重试for循环
			if attempt == maxRetries {
				break
			}

			// 走到这里就是上面返回错误，并且还未到最后一次重试，这里主要是记录重试日志
			utils.LogRetryAttempt(tryCloudCtx, fmt.Sprintf("集群-%s-页码-%d", instanceId, page), attempt, err, nil)

			// 指数退避
			delay := utils.CalculateDelay(attempt, initialDelay, maxDelay)

			// 先检查剩余时间是否足够
			if remaining, ok := utils.GetContextRemainingTime(ctx); ok {
				if remaining < delay {
					global.GVA_LOG.Warn("父context剩余时间不足",
						zap.String("reqID", reqID),
						zap.String("集群", instanceId),
						zap.Duration("剩余时间", remaining),
						zap.Duration("需要时间", delay))
					return fmt.Errorf("剩余时间不足: 需要%v但只剩%v", delay, remaining)
				}
			}

			if !utils.SleepWithContext(ctx, delay) {
				remaining, _ := utils.GetContextRemainingTime(ctx)
				global.GVA_LOG.Error("父context操作被取消",
					zap.String("reqID", reqID),
					zap.String("集群", instanceId),
					zap.Duration("取消时剩余时间", remaining),
					zap.Error(ctx.Err()))
				return fmt.Errorf("父级context操作被取消（查询实例慢查询方法）: %w", ctx.Err())
			}
		}

		// 最终错误检查
		if lastErr != nil {
			global.GVA_LOG.Error("查询ES慢日志失败(重试后)",
				zap.String("reqID", reqID),
				zap.String("集群", instanceId),
				zap.Int32("页码", page),
				zap.Error(lastErr))
			return fmt.Errorf("查询失败(尝试%d次): %w", maxRetries+1, lastErr)
		}

		// 处理结果
		if len(logs) == 0 {
			global.GVA_LOG.Info("ES实例无更多慢查询日志",
				zap.String("reqID", reqID),
				zap.String("集群", instanceId))
			break
		}

		// 统计信息
		totalLogs += len(logs)
		global.GVA_LOG.Info("获取慢日志",
			zap.String("reqID", reqID),
			zap.String("集群", instanceId),
			zap.Int32("页码", page), zap.Int("日志条数", len(logs)))

		// 处理每条日志
		for _, log := range logs {
			allLogs = append(allLogs, log)
		}

		// 5.7 检查是否还有下一页
		if int32(len(logs)) < pageSize {
			break
		}
		page++
	}

	// 每批处理多少条日志
	batchSizeConfig := global.GVA_CONFIG.AliyunEs.BatchSize
	batchSize := batchSizeConfig
	successCount := 0
	// 从0开始，每次循环增加batchSize
	for i := 0; i < len(allLogs); i += batchSize {
		// 当前批次的结束索引end，即当前批次开始索引i加上批次大小batchSize
		end := i + batchSize
		// 检查end是否超过了日志列表allLogs的长度。如果超过了将end设置为allLogs的长度，以确保不会超出数组边界
		if end > len(allLogs) {
			end = len(allLogs)
		}
		// 从allLogs中提取一个子切片batch，表示当前批次需要处理的日志
		batch := allLogs[i:end]

		// 调用AliProcessBatchLogs函数来处理当前批次的日志
		if err := AliProcessBatchLogs(ctx, esClient, batch, instanceId); err != nil {
			global.GVA_LOG.Warn("批量日志处理失败",
				zap.String("reqID", reqID),
				zap.String("集群", instanceId),
				zap.Int("批次起始", i),
				zap.Int("批次大小", len(batch)),
				zap.Error(err))
		} else {
			successCount += len(batch)
		}
	}

	// 6. 记录完成信息
	global.GVA_LOG.Info("ES实例查询完成",
		zap.String("reqID", reqID),
		zap.String("集群", instanceId),
		zap.Int("总条数", successCount))
	return nil
}

// AliProcessBatchLogs 处理单条日志
// @param context 上下文
// @param esClient 客户端
// @param logEntry 单条日志记录
// @return error 错误
func AliProcessBatchLogs(ctx context.Context, esClient *elasticsearch.Client, logEntries []interface{}, instanceId string) error {

	// 记录方法开始执行的时间点
	start := time.Now()
	reqID, _ := ctx.Value(utils.RequestIDKey).(string)

	// 初始化成功和失败计数器
	var successCount, failCount int

	// 定义一个defer函数，在方法结束时打印处理耗时信息
	defer func() {
		duration := time.Since(start)
		global.GVA_LOG.Info("AliProcessBatchLogs方法处理耗时",
			zap.String("reqID", reqID),
			zap.String("集群", instanceId),
			zap.Int("总日志数", len(logEntries)),
			zap.Int("成功数", successCount),
			zap.Int("失败数", failCount),
			zap.Int64("耗时(毫秒)", duration.Milliseconds()))
	}()

	// 将传入的日志条目解析为结构体
	var docs []esModel.AliEsSlowSearchLog
	for _, entry := range logEntries {
		doc, err := BuildDocument(ctx, entry)
		if err != nil {
			// 如果解析过程中出现错误，记录错误并增加失败计数
			global.GVA_LOG.Error("解析日志失败",
				zap.String("reqID", reqID),
				zap.Error(err))
			failCount++
			continue
		}
		docs = append(docs, *doc)
	}

	// 如果没有有效的日志文档，直接返回
	if len(docs) == 0 {
		return nil
	}

	// 根据当前日期生成索引名称
	today := time.Now().Format("2006-01-02")
	indexName := fmt.Sprintf("ali_es_slow_search_log-%s", today)

	// 构建Bulk API请求体
	var bulkBody bytes.Buffer
	for _, doc := range docs {
		id := AliBuildDocID(doc.Timestamp, doc.NodeIp, doc.IndexName, doc.DurationMs, doc.InstanceId)

		// 创建元数据（包含文档ID和索引名）
		// 键为字符串，值为任意类型的映射
		meta := map[string]interface{}{
			"update": map[string]interface{}{
				"_id":    id,
				"_index": indexName,
			},
		}
		// 序列化为 JSON
		metaBytes, err := json.Marshal(meta)
		if err != nil {
			// 如果序列化元数据失败，记录错误并增加失败计数
			failCount++
			global.GVA_LOG.Error("序列化元数据失败",
				zap.String("reqID", reqID),
				zap.Error(err),
				zap.Any("文档", doc))
			continue
		}

		// 创建更新操作的具体内容（使用Painless脚本进行更新或插入）
		body := map[string]interface{}{
			"script": map[string]interface{}{
				"source": "ctx._source = params.newData",
				"lang":   "painless",
				"params": map[string]interface{}{
					"newData": doc,
				},
			},
			"upsert": doc,
		}
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			// 如果序列化请求体失败，记录错误并增加失败计数
			failCount++
			global.GVA_LOG.Error("序列化请求体失败",
				zap.String("reqID", reqID),
				zap.Error(err),
				zap.Any("文档", doc))
			continue
		}

		bulkBody.Write(metaBytes)
		bulkBody.WriteString("\n")
		bulkBody.Write(bodyBytes)
		bulkBody.WriteString("\n")
	}

	// 判断Bulk API请求体是否为空
	if bulkBody.Len() == 0 {
		return fmt.Errorf("无法构建有效的批量请求体")
	}

	// 开始请求ES执行Bulk请求
	const (
		maxRetries    = 2               // 最大重试次数
		initialDelay  = 1 * time.Second // 初始延迟
		maxDelay      = 10 * time.Second
		perTryTimeout = 30 * time.Second // 单次尝试超时
	)
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		// 检查上下文是否已被取消
		if ctx.Err() != nil {
			return fmt.Errorf("在发起 Bulk 请求前 context 已被取消: %w", ctx.Err())
		}

		// 创建带超时的子context
		writeCtx, cancel := context.WithTimeout(ctx, perTryTimeout)

		// 执行Bulk请求
		res, err := esClient.Bulk(
			bytes.NewReader(bulkBody.Bytes()),
			esClient.Bulk.WithContext(writeCtx),
			esClient.Bulk.WithIndex(indexName),
		)
		cancel()

		// 检查响应结果和错误，赋值给respErr变量
		var respErr error
		switch {
		case err != nil:
			respErr = fmt.Errorf("bulk请求发送失败: %w", err)
		case res == nil:
			respErr = errors.New("ES返回空响应")
		case res.IsError():
			respErr = fmt.Errorf("ES错误[%d]: %s", res.StatusCode, res.String())
		}

		// 检查
		if respErr != nil {
			lastErr = respErr
			// 当前的重试次数 (attempt) 来计算下一次重试之前应该等待/延时的时间
			if attempt < maxRetries {
				delay := utils.CalculateDelay(attempt, initialDelay, maxDelay)
				global.GVA_LOG.Warn("Bulk写入失败，即将重试",
					zap.String("reqID", reqID),
					zap.Int("尝试次数", attempt+1),
					zap.Duration("延迟", delay),
					zap.Error(respErr))

				// 指定的延迟时间内休眠，同时监听上下文的取消或超时信号
				if !utils.SleepWithContext(ctx, delay) {
					return fmt.Errorf("context被取消: %w", ctx.Err())
				}
				// 跳出重试for的循环的本次循环
				continue
			}
			// 判断有超过最大重试次数，终止for的整体循环
			break
		}

		// 成功处理后，计算成功数并返回
		successCount = len(docs) - failCount
		return nil
	}

	// 如果所有尝试均失败，则标记所有文档为失败，并记录错误信息
	failCount = len(docs) // 标记所有文档为失败
	global.GVA_LOG.Error("批量写入失败",
		zap.String("reqID", reqID),
		zap.String("索引名", indexName),
		zap.Int("尝试次数", maxRetries+1),
		zap.Error(lastErr))
	return fmt.Errorf("批量写入失败(共尝试%d次): %w", maxRetries+1, lastErr)
}

// AliBuildDocID 构建复合唯一键
// @param timestamp 时间戳
// @param nodeIP 节点 IP 地址
// @param indexName 索引名称
// @param durationMs 持续时间(毫秒)
// @param instanceID 实例 ID
// @return string 文档唯一 ID
func AliBuildDocID(timestamp int64, nodeIP, indexName string, durationMs float64, instanceID string) string {
	return fmt.Sprintf("%d|%s|%s|%d|%s",
		timestamp,
		strings.ReplaceAll(nodeIP, ".", "_"),
		indexName,
		int(durationMs),
		instanceID,
	)
}

// BuildDocument 构建文档
// @param logEntry 单条日志记录
// @return AliEsSlowSearchLog 结构体
// @return error 错误
func BuildDocument(ctx context.Context, logEntry interface{}) (*esModel.AliEsSlowSearchLog, error) {
	reqID, _ := ctx.Value(utils.RequestIDKey).(string)
	var resp esResp.AliSlowSearchLogResponse
	jsonData, err := json.Marshal(logEntry)
	if err != nil {
		return nil, fmt.Errorf("序列化日志失败: %w", err)
	}
	if err := json.Unmarshal(jsonData, &resp); err != nil {
		return nil, fmt.Errorf("反序列化日志失败: %w", err)
	}

	searchTimeMs, _ := strconv.ParseFloat(resp.ContentCollection.SearchTimeMs, 64)
	totalHits, _ := strconv.ParseFloat(resp.ContentCollection.SearchTotalHits, 64)
	shardId, _ := strconv.ParseInt(resp.ContentCollection.ShardID, 10, 64)
	shardTotal, _ := strconv.ParseInt(resp.ContentCollection.TotalShards, 10, 64)

	t, err := time.Parse("2006-01-02T15:04:05,000", resp.ContentCollection.Time)
	if err != nil {
		return nil, fmt.Errorf("解析时间失败: %w", err)
	}
	formattedTime := t.Format("2006-01-02T15:04:05.000Z")

	// 使用流式（非EncodeToken）脱敏，可以保证原始顺序，如果反序列化到map保证不了顺序（map遍历顺序是不确定的）
	maskedStr, hashStr, dslFormat, err := utils.MaskAnyStream(resp.ContentCollection.Content, resp.InstanceID, resp.ContentCollection.IndexName, reqID)
	if err != nil {
		global.GVA_LOG.Error("对阿里云ES慢查询DSL脱敏处理异常",
			zap.String("reqID", reqID),
			zap.Error(err))
		return nil, err
	}
	queryTemplate := maskedStr
	checkSum := hashStr

	doc := &esModel.AliEsSlowSearchLog{
		EventTime:      formattedTime,
		Timestamp:      resp.Timestamp,
		QueryText:      resp.ContentCollection.Content,
		QueryTemplate:  queryTemplate,
		CheckSum:       checkSum,
		DslFormat:      dslFormat,
		DurationMs:     searchTimeMs,
		IndexName:      resp.ContentCollection.IndexName,
		NodeIp:         resp.ContentCollection.Host,
		InstanceId:     resp.InstanceID,
		Level:          resp.ContentCollection.Level,
		SearchHits:     totalHits,
		SearchType:     resp.ContentCollection.SearchType,
		ShardId:        shardId,
		SlowSearchType: resp.ContentCollection.SlowSearchLogType,
		ShardTotal:     shardTotal,
		TaskId:         reqID,
	}

	return doc, nil
}

// AliyunEsSlowLogs 程序入口
// @return error 错误
func AliyunEsSlowLogs() (_err error) {

	// 1、创建一个Context（带超时），通过函数参数层层传递下去，以便在任何地方都能感知到取消信号或截止时间
	aliContextTimeout := global.GVA_CONFIG.AliyunEs.ContextTimeout
	reqID := uuid.New().String()

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(aliContextTimeout)*time.Minute)
	ctx = context.WithValue(ctx, utils.RequestIDKey, reqID)
	defer cancel()

	// 2、初始化阿里云
	aliyunClient, _err := AliEsClientInit()
	if _err != nil {
		global.GVA_LOG.Error("初始化阿里云:执行失败: ",
			zap.String("reqID", reqID),
			zap.Error(_err))
		return _err
	} else {
		global.GVA_LOG.Info("初始化阿里云:执行完成",
			zap.String("reqID", reqID))
	}

	// 3、获取ES慢日志
	_, _err = GetAliyunEsSlowLogs(ctx, aliyunClient)
	if _err != nil {
		global.GVA_LOG.Error("调用阿里云ES慢日志接口:执行失败: ",
			zap.String("reqID", reqID),
			zap.Error(_err))
		return _err
	} else {
		global.GVA_LOG.Info("调用阿里云ES慢日志接口:执行完成",
			zap.String("reqID", reqID))
	}

	return _err
}
