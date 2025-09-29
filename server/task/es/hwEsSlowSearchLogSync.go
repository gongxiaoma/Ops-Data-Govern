package es

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/flipped-aurora/gin-vue-admin/server/global"
	esModel "github.com/flipped-aurora/gin-vue-admin/server/model/es"
	"github.com/flipped-aurora/gin-vue-admin/server/utils"
	"github.com/google/uuid"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	css "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/css/v1"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/services/css/v1/model"
	region "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/css/v1/region"
	"github.com/olivere/elastic/v7"
	"go.uber.org/zap"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// HwCssClusterError 记录实例级别的错误信息
type HwCssClusterError struct {
	InstanceID string
	Error      error
}

// HwyunSdkInit 华为云SDK初始化
// @param ak 华为云ak
// @param sk 华为云sk
// @param regionStr 区域
// @return CssClient 华为云ES客户端
// @return error 错误
func HwyunSdkInit(accessKeyId *string, accessKeySecret *string, regionId *string) (*css.CssClient, error) {

	// 1. 安全获取区域值
	regionValue, err := region.SafeValueOf(*regionId)
	if err != nil {
		return nil, fmt.Errorf("无效的区域字符串'%s': %w", regionId, err)
	}

	// 2. 创建认证凭证
	auth, err := basic.NewCredentialsBuilder().
		WithAk(*accessKeyId).
		WithSk(*accessKeySecret).
		SafeBuild()
	if err != nil {
		global.GVA_LOG.Error("初始化华为云:执行失败: ", zap.Error(err))
		return nil, fmt.Errorf("认证初始化失败: %w", err)
	}

	// 3. 创建CSS客户端(使用v1包的NewCssClient)
	hwEsClient := css.NewCssClient(
		css.CssClientBuilder().
			WithRegion(regionValue).
			WithCredential(auth).
			Build())

	return hwEsClient, nil
}

// HwCssClientInit 华为云ES客户端初始化
// @return CssClient 华为云ES客户端
// @return error 错误
func HwCssClientInit() (hwEsClient *css.CssClient, _err error) {

	// 华为云密钥
	hwyunAccessKeyIdCipherText := global.GVA_CONFIG.HwyunEs.HwyunKey
	hwyunAccessKeySecretCipherText := global.GVA_CONFIG.HwyunEs.HwyunSecret
	regionId := global.GVA_CONFIG.HwyunEs.Region

	cipherTextsMap := map[string]string{
		"hwyunAccessKeyId":     hwyunAccessKeyIdCipherText,
		"hwyunAccessKeySecret": hwyunAccessKeySecretCipherText,
	}

	cipherKeyPlainText := global.GVA_CONFIG.HwyunEs.CipherKey
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

	hwyunAccessKeyId := decryptedMap["hwyunAccessKeyId"]
	hwyunAccessKeySecret := decryptedMap["hwyunAccessKeySecret"]

	// 初始华为云化客户端
	hwEsClient, _err = HwyunSdkInit(&hwyunAccessKeyId, &hwyunAccessKeySecret, &regionId)
	if _err != nil {
		global.GVA_LOG.Error("初始化华为云SDK:执行失败: ", zap.Error(_err))
		return nil, _err
	} else {
		global.GVA_LOG.Info("初始化华为云SDK:执行完成")
	}

	return hwEsClient, _err
}

// GetHwyunEsSlowLogs 获取华为云ES慢查询
// @param client 客户端
// @return error 错误
func GetHwyunEsSlowLogs(ctx context.Context, hwEsClient *css.CssClient) ([]HwCssClusterError, error) {

	// 方法执行开始时间
	start := time.Now()
	reqID, _ := ctx.Value(utils.RequestIDKey).(string)

	clusterNode := global.GVA_CONFIG.Es.ClusterNode
	userName := global.GVA_CONFIG.Es.UserName
	passWord := global.GVA_CONFIG.Es.PassWord

	// 初始化ES客户端
	esClient, err := elastic.NewClient(
		elastic.SetURL(clusterNode...),
		elastic.SetBasicAuth(userName, passWord),
		elastic.SetSniff(false),
		elastic.SetHealthcheckTimeoutStartup(10*time.Second),
		elastic.SetHealthcheckTimeout(5*time.Second),
	)
	if err != nil {
		global.GVA_LOG.Error("ES连接失败: ",
			zap.String("reqID", reqID),
			zap.Error(err))
		log.Fatal("ES连接失败: ", err)
	}
	defer esClient.Stop()

	// 从配置获取ES实例ID列表
	hwClusterIds := global.GVA_CONFIG.HwyunEs.EsList
	if len(hwClusterIds) == 0 {
		errMsg := "配置文件中未找到elasticsearch实例id列表"
		global.GVA_LOG.Error(errMsg,
			zap.String("reqID", reqID))
		return nil, nil
	}

	// 并发控制初始化
	maxConcurrency := global.GVA_CONFIG.HwyunEs.MaxConcurrency
	var (
		wg        sync.WaitGroup                                    // 主线程等待一组goroutine的执行完毕
		hwErrChan = make(chan HwCssClusterError, len(hwClusterIds)) // 管道
		sem       = make(chan struct{}, maxConcurrency)             // sem主要用于控制并发
	)

	for _, clusterId := range hwClusterIds {
		currentID := clusterId // 创建局部副本

		wg.Add(1)         // 通常写在启动新的goroutine之前
		sem <- struct{}{} // 获取信号量

		go func() {
			defer func() {
				<-sem     // 释放信号量
				wg.Done() // 通常写在goroutine之中
			}()

			// 获取集群详情
			clusterDetail, err := HwGetClusterDetail(hwEsClient, currentID)
			if err != nil {
				global.GVA_LOG.Error("获取集群详情失败: ",
					zap.String("reqID", reqID),
					zap.Error(err))
				fmt.Printf("获取集群详情失败: %v\n", err)
			}

			// 提取节点列表
			nodes := HwExtractNodes(clusterDetail)
			fmt.Printf("集群 %s 共有 %d 个节点: %v\n",
				*clusterDetail.Name, len(nodes), nodes)

			for _, nodeName := range nodes {
				// 处理单个实例
				if err := HwProcessNodeSlowLogs(ctx, hwEsClient, esClient, clusterId, nodeName); err != nil {
					// 处理单个实例，如果有返回错误，写入到channel中
					hwErrChan <- HwCssClusterError{
						InstanceID: currentID,
						Error:      err,
					}
				}
			}
		}()
	}

	// 单独启动goroutine等待和关闭，否则可能会阻塞主线程下面的代码
	go func() {
		wg.Wait()        // 等待所有goroutine完成
		close(hwErrChan) // 只有当所有工作goroutine都调用了wg.Done()之后才会执行
	}()

	// 收集错误（上面单独启动goroutine，就算阻塞如下代码依然可以执行）
	var errorList []HwCssClusterError
	for e := range hwErrChan {
		global.GVA_LOG.Error("实例处理失败",
			zap.String("reqID", reqID),
			zap.String("instanceId", e.InstanceID),
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
		global.GVA_LOG.Info("GetHwyunEsSlowLogs方法处理耗时",
			zap.String("reqID", reqID),
			zap.String("reqID", reqID),
			zap.Int64("耗时(毫秒)", durationMillis))
	}()
	return nil, nil
}

// HwProcessNodeSlowLogs 处理单个实例慢日志
// @param context 时间戳（毫秒）
// @param client 华为云ES客户端（用于查询数据）
// @param esClient 本地ES客户端（用于写入数据）
// @param instanceId 持续时间（毫秒）
// @param startTime 实例 ID
// @param endTime 实例 ID
// @return error 错误
func HwProcessNodeSlowLogs(ctx context.Context, hwEsClient *css.CssClient, esClient *elastic.Client,
	clusterId string, nodeName string) error {

	// 统计方法执行时间
	start := time.Now()
	reqID, _ := ctx.Value(utils.RequestIDKey).(string)
	defer func() {
		global.GVA_LOG.Info("实例处理完成",
			zap.String("reqID", reqID),
			zap.String("集群", clusterId),
			zap.String("节点", nodeName),
			zap.Duration("耗时（毫秒）", time.Since(start)))
	}()

	// 重试配置
	const (
		maxRetries    = 2
		initialDelay  = 1 * time.Second
		maxDelay      = 10 * time.Second
		perTryTimeout = 30 * time.Second
	)

	// 初始化分页参数
	var (
		page int32 = 1
	)

	// 记录开始信息
	global.GVA_LOG.Info("开始查询华为云ES实例慢查询日志",
		zap.String("reqID", reqID),
		zap.String("集群", clusterId),
		zap.String("节点", nodeName))

	var lastErr error
	var logs []model.LogList

	// 重试循环
	for attempt := 0; attempt <= maxRetries; attempt++ {
		// 用于控制重试的context单独设置超时时间，和父context没有关系
		tryCloudCtx, tryCloudCancel := context.WithTimeout(context.Background(), perTryTimeout)

		err := func() error {
			defer tryCloudCancel()
			// 构建请求
			request := &model.ShowLogBackupRequest{
				ClusterId: clusterId,
				Body: &model.GetLogBackupReq{
					LogType:      "searchSlow",
					Level:        "WARN",
					InstanceName: nodeName,
				},
			}

			// 发送请求
			response, err := hwEsClient.ShowLogBackup(request)
			if err != nil {
				global.GVA_LOG.Error("查询ES慢日志失败(实例ID: %s): %v",
					zap.String("reqID", reqID),
					zap.String("集群", clusterId),
					zap.String("节点", nodeName), zap.Error(err))
				return fmt.Errorf("查询ES慢日志失败: %w", err)
			}

			// 判断返回数据是否为空
			if response == nil || response.LogList == nil {
				global.GVA_LOG.Info("ES实例无更多慢查询日志",
					zap.String("reqID", reqID),
					zap.String("集群", clusterId),
					zap.String("节点", nodeName))
				return fmt.Errorf("响应数据为空")
			}

			logs = *response.LogList
			return nil
		}()

		if err == nil {
			// 成功时记录是否经过重试
			if attempt > 0 {
				global.GVA_LOG.Warn("查询ES慢日志成功(经过重试)",
					zap.String("reqID", reqID),
					zap.Int("第几次重试", attempt),
					zap.String("集群", clusterId))
			}
			break
		}

		lastErr = err
		// 最后一次尝试不等待
		if attempt == maxRetries {
			break
		}

		// 记录重试
		utils.LogRetryAttempt(tryCloudCtx, fmt.Sprintf("集群-%s-页码-%d", clusterId, page), attempt, err, nil)

		// 指数退避
		delay := utils.CalculateDelay(attempt, initialDelay, maxDelay)
		if !utils.SleepWithContext(ctx, delay) {
			return fmt.Errorf("操作被取消: %w", ctx.Err())
		}
	}

	// 最终错误检查
	if lastErr != nil {
		global.GVA_LOG.Error("查询ES慢日志失败(重试后)",
			zap.String("reqID", reqID),
			zap.String("集群", clusterId),
			zap.String("节点", nodeName),
			zap.Int32("页码", page),
			zap.Error(lastErr))
		return fmt.Errorf("查询失败(尝试%d次): %w", maxRetries+1, lastErr)
	}

	// 防御性处理nil
	if logs == nil {
		logs = []model.LogList{}
	}

	// 处理每条日志
	for _, logEntry := range logs {
		if err := HwProcessSingleLog(ctx, clusterId, esClient, logEntry); err != nil {
			global.GVA_LOG.Warn("单条日志处理失败",
				zap.String("reqID", reqID),
				zap.String("集群", clusterId),
				zap.String("节点", nodeName),
				zap.Any("logEntry", logEntry),
				zap.Error(err))
			continue
		}
	}

	// 记录完成信息
	global.GVA_LOG.Info("ES实例查询完成",
		zap.String("reqID", reqID),
		zap.String("集群", clusterId),
		zap.String("节点", nodeName),
		zap.Int("总条数", len(logs)))
	return nil
}

// HwBuildDocID 构建复合唯一键
// @param timestamp 时间戳
// @param nodeIP 节点 IP 地址
// @param indexName 索引名称
// @param durationMs 持续时间（毫秒）
// @param instanceID 实例 ID
// @return string 文档唯一 ID
func HwBuildDocID(timestamp string, nodeName string, indexName string, durationMs float64, clusterID string) string {
	return fmt.Sprintf("%s|%s|%s|%d|%s",
		timestamp,
		nodeName,
		indexName,
		int(durationMs),
		clusterID,
	)
}

// HwProcessSingleLog 处理单条日志
// @param esClient 客户端
// @param logEntry 单条日志记录
// @return error 错误
func HwProcessSingleLog(ctx context.Context, clusterId string, esClient *elastic.Client, logList model.LogList) error {

	// 记录方法开始执行的时间点
	start := time.Now()
	reqID, _ := ctx.Value(utils.RequestIDKey).(string)

	// 提前声明变量
	var nodeName string
	var indexName string
	var shardID int64
	var durationMs float64
	var searchHits int64
	var searchType string
	var shardTotal int64
	var queryText string

	logContent := *logList.Content
	timeStamp := logList.Date
	logLevel := logList.Level

	// 解析时间
	t, err := time.Parse("2006-01-02T15:04:05,000", *timeStamp)
	if err != nil {
		global.GVA_LOG.Error("解析时间失败: ",
			zap.String("reqID", reqID),
			zap.Error(err))
		return fmt.Errorf("解析时间失败: %w", err)
	}
	formattedTime := t.Format("2006-01-02T15:04:05.000Z")
	unixMilliTimestamp := t.UnixMilli()

	// 提取节点名、索引名和分片ID
	mainRegex := regexp.MustCompile(`\[([^\]]+)\]\s+\[([^\]]+)\]\s+\[([^\]]+)\]\[(\d+)\]`)
	if matches := mainRegex.FindStringSubmatch(logContent); len(matches) >= 5 {
		nodeName = strings.TrimSpace(matches[2])
		indexName = strings.TrimSpace(matches[3])
		shardID, _ = strconv.ParseInt(matches[4], 10, 64)
	}

	// 提取耗时（毫秒）
	if matches := regexp.MustCompile(`took_millis\[(\d+)\]`).FindStringSubmatch(logContent); len(matches) >= 2 {
		durationMs, _ = strconv.ParseFloat(matches[1], 64)
	}

	// 提取命中数量
	if matches := regexp.MustCompile(`total_hits\[(\d+) hits\]`).FindStringSubmatch(logContent); len(matches) >= 2 {
		searchHits, _ = strconv.ParseInt(matches[1], 10, 64)
	}

	// 提取查询类型
	if matches := regexp.MustCompile(`search_type\[([^\]]+)\]`).FindStringSubmatch(logContent); len(matches) >= 2 {
		searchType = matches[1]
	}

	// 提取分片总数
	if matches := regexp.MustCompile(`total_shards\[(\d+)\]`).FindStringSubmatch(logContent); len(matches) >= 2 {
		shardTotal, _ = strconv.ParseInt(matches[1], 10, 64)
	}

	// 从日志内容提取json格式dsl内容，第2个返回值obj是map[string]interface{}，可直接访问其中的字段，如obj["query"]
	queryText, _, err = HwExtractDsl(logContent)
	if err != nil {
		global.GVA_LOG.Error("从日志内容提取json格式dsl内容失败",
			zap.String("reqID", reqID),
			zap.Error(err))
		return err
	}

	// 使用流式（非EncodeToken）脱敏，可以保证原始顺序，如果反序列化到map保证不了顺序（map遍历顺序是不确定的）
	maskedStr, hashStr, dslFormat, err := utils.MaskAnyStream(queryText, clusterId, indexName, reqID)
	if err != nil {
		global.GVA_LOG.Error("对华为云ES慢查询DSL脱敏处理异常",
			zap.String("reqID", reqID),
			zap.Error(err))
		return err
	}
	queryTemplate := maskedStr
	checkSum := hashStr

	// 构建ES文档
	doc := esModel.HwEsSlowSearchLog{
		EventTime:     formattedTime,
		Timestamp:     unixMilliTimestamp,
		QueryText:     queryText,
		QueryTemplate: queryTemplate,
		CheckSum:      checkSum,
		DslFormat:     dslFormat,
		DurationMs:    durationMs,
		IndexName:     indexName,
		NodeName:      nodeName,
		InstanceId:    clusterId,
		Level:         *logLevel,
		SearchHits:    searchHits,
		SearchType:    searchType,
		ShardId:       shardID,
		ShardTotal:    shardTotal,
		TaskId:        reqID,
	}

	// 构建带日期的索引
	today := time.Now().Format("2006-01-02")
	hwSlowSearchLogIndexName := fmt.Sprintf("hw_es_slow_search_log-%s", today)

	// 生成文档ID
	id := HwBuildDocID(doc.EventTime, doc.NodeName, doc.IndexName, doc.DurationMs, doc.InstanceId)

	// 首次写入或获取文档的当前版本信息
	script := elastic.NewScriptInline("ctx._source = params.newData").Lang("painless").
		Param("newData", doc)

	// 重试配置
	const (
		maxRetries    = 2               // 最大重试次数
		initialDelay  = 1 * time.Second // 初始延迟
		maxDelay      = 10 * time.Second
		perTryTimeout = 30 * time.Second // 单次尝试超时
	)

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		// 用于控制重试的context单独设置超时时间，和父context没有关系
		tryCtx, tryCancel := context.WithTimeout(context.Background(), perTryTimeout)

		// 拉取阿里云接口数据使用go-elasticsearch v7进行写入ES，拉取华为云接口数据使用olivere/elastic v7进行写入ES，目的熟悉各个库
		_, err = esClient.Update().
			Index(hwSlowSearchLogIndexName).
			Id(id).
			Script(script).
			RetryOnConflict(3). // 可选：重试冲突
			Upsert(doc).        // 如果文档不存在，则插入这个 doc
			Do(tryCtx)
		tryCancel() // 立即释放资源

		if err == nil {
			// 成功时记录是否经过重试
			if attempt > 0 {
				global.GVA_LOG.Warn("实例处理成功(经过重试)",
					zap.String("reqID", reqID),
					zap.Int("第几次重试", attempt),
					zap.Any("文档信息", doc)) // 明确记录重试次数
			}
			return nil
		}

		lastErr = err
		// 上面未返回错误，正常会return。所以走到这一步，就是表示有错误，然后判断是不是达到最大重试次数，是直接跳出
		if attempt == maxRetries {
			break
		}

		// 记录重试信息（只有非最后一次才显示"即将重试"）
		if attempt < maxRetries {
			utils.LogRetryAttempt(tryCtx, hwSlowSearchLogIndexName, attempt, err, doc)
			delay := utils.CalculateDelay(attempt, initialDelay, maxDelay) // 直接传attempt
			if !utils.SleepWithContext(ctx, delay) {
				return fmt.Errorf("操作被取消: %w", ctx.Err())
			}
		}
	}

	// 定义一个defer函数，在方法结束时打印处理耗时信息
	defer func() {
		duration := time.Since(start)
		global.GVA_LOG.Info("HwProcessSingleLog方法处理耗时",
			zap.String("reqID", reqID),
			zap.String("集群", clusterId),
			zap.String("节点", nodeName),
			zap.String("时间", formattedTime),
			zap.Int64("耗时(毫秒)", duration.Milliseconds()))
	}()

	// 最终失败日志（记录总重试次数）
	global.GVA_LOG.Error("ES写入失败（重试失败）",
		zap.String("待写入索引", hwSlowSearchLogIndexName),
		zap.String("reqID", reqID),
		zap.Int("总尝试次数", maxRetries+1),
		zap.Int("总重试次数", maxRetries),
		zap.Error(lastErr),
		zap.Any("文档信息", doc))
	return fmt.Errorf("ES写入失败(共尝试%d次，其中重试%d次): %w", maxRetries+1, maxRetries, lastErr)
}

// HwGetClusterDetail 获取华为云ES集群详情
// @param esClient 客户端
// @param clusterId 集群ID
// @return ShowClusterDetailResponse 华为云ES集群详情
// @return error 错误
func HwGetClusterDetail(hwEsClient *css.CssClient, clusterId string) (*model.ShowClusterDetailResponse, error) {
	request := &model.ShowClusterDetailRequest{}
	request.ClusterId = clusterId
	return hwEsClient.ShowClusterDetail(request)
}

// HwExtractNodes 获取华为云ES集群详情
// @param ShowClusterDetailResponse 华为云ES集群详情
// @return []string
func HwExtractNodes(response *model.ShowClusterDetailResponse) []string {
	var nodes []string
	if response.Instances != nil {
		for _, instance := range *response.Instances {
			if instance.Name != nil && *instance.Type == "ess" {
				nodes = append(nodes, *instance.Name)
			}
		}
	}
	return nodes
}

// HwExtractDsl 获取华为云ES集群详情
// @param logContent 日志原始结构
// @return string
// @return 对象
func HwExtractDsl(logContent string) (string, map[string]interface{}, error) {
	// 找到 source[
	start := strings.Index(logContent, "source[")
	if start == -1 {
		return "", nil, fmt.Errorf("no source[ found")
	}
	start += len("source[")

	sub := logContent[start:]

	// 使用 json.RawMessage 获取原始 JSON
	var raw json.RawMessage
	dec := json.NewDecoder(strings.NewReader(sub))
	if err := dec.Decode(&raw); err != nil {
		return "", nil, fmt.Errorf("json decode failed: %w", err)
	}

	// raw 本身就是原始 JSON 字符串
	jsonPart := string(raw)

	// 解析成 map[string]interface{}
	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return "", nil, fmt.Errorf("json unmarshal to map failed: %w", err)
	}

	return jsonPart, obj, nil
}

// HwyunEsSlowLogs 程序入口
// @return error 错误
func HwyunEsSlowLogs() (_err error) {

	// 1. 创建一个Context（带超时），通过函数参数层层传递下去，以便在任何地方都能感知到取消信号或截止时间
	hwContextTimeout := global.GVA_CONFIG.HwyunEs.ContextTimeout
	reqID := uuid.New().String()

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(hwContextTimeout)*time.Minute)
	ctx = context.WithValue(ctx, utils.RequestIDKey, reqID)
	defer cancel()

	// 2、初始化华为云
	hwyunClient, _err := HwCssClientInit()
	if _err != nil {
		global.GVA_LOG.Error("初始化华为云:执行失败: ",
			zap.String("reqID", reqID),
			zap.Error(_err))
		return _err
	} else {
		global.GVA_LOG.Info("初始化华为云:执行完成",
			zap.String("reqID", reqID))
	}

	// 3、获取ES慢日志
	_, _err = GetHwyunEsSlowLogs(ctx, hwyunClient)
	if _err != nil {
		global.GVA_LOG.Error("调用华为云ES慢日志接口:执行失败: ",
			zap.String("reqID", reqID),
			zap.Error(_err))
		return _err
	} else {
		global.GVA_LOG.Info("调用华为云ES慢日志接口:执行完成",
			zap.String("reqID", reqID))
	}

	return _err
}
