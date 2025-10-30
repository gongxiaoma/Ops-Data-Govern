package es

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/flipped-aurora/gin-vue-admin/server/global"
	"github.com/olivere/elastic/v7"
	"go.uber.org/zap"
	"math"
	"net/http"
	"time"
)

// SlowSearchResult 慢查询日志返回结果结构体
type SlowSearchResult struct {
	Brand                     string  `json:"brand"`
	DBName                    string  `json:"DBName"`
	SQLText                   string  `json:"SQLText"`
	SQLHash                   string  `json:"sqlhash"`
	SQLTime                   string  `json:"sqltime"`
	Time                      string  `json:"__time__"`
	Source                    string  `json:"__source__"`
	DBInstanceName            string  `json:"DBInstanceName"`
	MaxExecutionTime          float64 `json:"MaxExecutionTime"`
	AverageExecutionTime      float64 `json:"AverageExecutionTime"`
	MySQLTotalExecutionCounts int64   `json:"MySQLTotalExecutionCounts"`
}

// InspectionRequest 巡检上报结构体
type InspectionRequest struct {
	Date             string           `json:"Date"`
	BusinessType     string           `json:"BusinessType"`
	InspectionType   string           `json:"InspectionType"`
	InspectionName   string           `json:"InspectionName"`
	InspectionResult SlowSearchResult `json:"InspectionResult"`
}

// ReadESClient 初始化 ES 客户端
// @return es客户端
// @return error 错误
func ReadESClient() (*elastic.Client, error) {
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
		if global.GVA_LOG != nil {
			global.GVA_LOG.Error("ES连接失败: ", zap.Error(err))
		}
		return nil, err
	}
	return client, nil
}

// ReportSlowSearch 巡检上报接口
// @param result 单条慢查询
// @return error 错误
func ReportSlowSearch(result SlowSearchResult) error {
	// 当前时间
	currentTime := time.Now().Format("2006-01-02 15:04:05")

	// 构造请求体
	reqData := InspectionRequest{
		Date:             currentTime,
		BusinessType:     global.GVA_CONFIG.EsInspection.SdBrand,
		InspectionType:   global.GVA_CONFIG.EsInspection.InspectionType,
		InspectionName:   global.GVA_CONFIG.EsInspection.InspectionName,
		InspectionResult: result,
	}

	// 序列化
	payload, err := json.Marshal(reqData)
	if err != nil {
		global.GVA_LOG.Error("巡检上报接口入参序列化失败", zap.Error(err))
		return fmt.Errorf("JSON 序列化失败: %v", err)
	}

	// 请求头
	client := &http.Client{Timeout: 10 * time.Second}
	inspectionUrl := global.GVA_CONFIG.EsInspection.InspectionUrl
	inspectionBearer := global.GVA_CONFIG.EsInspection.InspectionBearer
	req, err := http.NewRequest("POST", inspectionUrl, bytes.NewBuffer(payload))
	if err != nil {
		global.GVA_LOG.Error("巡检上报接口连接失败", zap.Error(err))
		return fmt.Errorf("创建请求失败: %v", err)
	}
	req.Header.Set("Authorization", inspectionBearer)
	req.Header.Set("Content-Type", "application/json")

	// 发送请求
	resp, err := client.Do(req)
	if err != nil {
		global.GVA_LOG.Error("巡检上报接口请求发送失败", zap.Error(err))
		return fmt.Errorf("请求发送失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		global.GVA_LOG.Error("巡检上报接口上报失败", zap.Error(err))
		return fmt.Errorf("上报失败，HTTP Status: %d", resp.StatusCode)
	}
	return nil
}

// FetchSlowSearchAgg 聚合查询并将所有慢请求封装返回
// @param client ES客户端
// @return []SlowSearchResult 所有慢请求封装返回
// @return error 错误
func FetchSlowSearchAgg(client *elastic.Client) ([]SlowSearchResult, error) {
	ctx := context.Background()

	// 计算前一天索引
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	indices := []struct {
		Name  string
		Brand string
		Query elastic.Query
	}{
		{
			Name:  fmt.Sprintf("ali_es_slow_search_log-%s", yesterday),
			Brand: global.GVA_CONFIG.EsInspection.SdBrand,
			Query: elastic.NewRangeQuery("durationMs").Gte(global.GVA_CONFIG.EsInspection.AliSlowThreshold),
		},
		{
			Name:  fmt.Sprintf("hw_es_slow_search_log-%s", yesterday),
			Brand: global.GVA_CONFIG.EsInspection.JdBrand,
			Query: elastic.NewRangeQuery("durationMs").Gte(global.GVA_CONFIG.EsInspection.HwSlowThreshold),
		},
	}

	// 构造聚合（按checkSum分组，统计每个分桶文档数量和一条样本数据）
	// 定义一个切片用于返回多条查询数据（类似数值）
	var results []SlowSearchResult
	for _, idx := range indices {
		checkSumAgg := elastic.NewTermsAggregation().
			Field("checkSum").
			Size(10000).
			SubAggregation("max_duration", elastic.NewMaxAggregation().Field("durationMs")).
			SubAggregation("avg_duration", elastic.NewAvgAggregation().Field("durationMs")).
			SubAggregation("sample_doc", elastic.NewTopHitsAggregation().
				Size(1).
				FetchSourceContext(
					elastic.NewFetchSourceContext(true).
						Include("@timestamp", "instanceId", "indexName", "queryText"),
				))

		// 执行查询
		searchResult, err := client.Search().
			Index(idx.Name).
			Query(idx.Query).
			Size(0).
			Aggregation("by_checksum", checkSumAgg).
			Do(ctx)
		if err != nil {
			global.GVA_LOG.Error("执行聚合查询失败", zap.Error(err))
			return nil, err
		}

		// 解析聚合结果
		agg, found := searchResult.Aggregations.Terms("by_checksum")
		if !found {
			global.GVA_LOG.Error("未找到by_checksum聚合结果", zap.Error(err))
			return nil, fmt.Errorf("未找到by_checksum聚合结果")
		}
		for _, bucket := range agg.Buckets {
			checkSum := bucket.Key.(string)
			docCount := bucket.DocCount
			maxAgg, _ := bucket.Max("max_duration")
			avgAgg, _ := bucket.Avg("avg_duration")
			var maxDurMs, avgDurMs float64
			if maxAgg != nil && maxAgg.Value != nil {
				maxDurMs = *maxAgg.Value
			}
			if avgAgg != nil && avgAgg.Value != nil {
				avgDurMs = *avgAgg.Value
			}

			// 获取样本数据
			sampleDoc, _ := bucket.TopHits("sample_doc")
			var info map[string]interface{}
			if sampleDoc != nil && len(sampleDoc.Hits.Hits) > 0 {
				doc := sampleDoc.Hits.Hits[0]
				_ = json.Unmarshal(doc.Source, &info)
			}

			// 时间格式化
			var sqlTime string
			if ts, ok := info["@timestamp"].(float64); ok {
				t := time.UnixMilli(int64(ts))
				sqlTime = t.Format("2006-01-02 15:04:05")
			} else if tsStr, ok := info["@timestamp"].(string); ok {
				if t, err := time.Parse(time.RFC3339, tsStr); err == nil {
					sqlTime = t.Format("2006-01-02 15:04:05")
				} else {
					sqlTime = tsStr
				}
			}

			// 封装聚合数据到到结构体中
			result := SlowSearchResult{
				Brand:                     idx.Brand,
				DBName:                    fmt.Sprintf("%v", info["indexName"]),
				SQLText:                   fmt.Sprintf("%v", info["queryText"]),
				SQLHash:                   checkSum,
				SQLTime:                   sqlTime,
				Time:                      time.Now().Format("2006-01-02 15:04:05"),
				Source:                    "",
				DBInstanceName:            fmt.Sprintf("%v", info["instanceId"]),
				MaxExecutionTime:          math.Round(maxDurMs*100) / 100,
				AverageExecutionTime:      math.Round(avgDurMs*100) / 100,
				MySQLTotalExecutionCounts: docCount,
			}
			// 往切片里面追加数据
			results = append(results, result)
		}
	}
	return results, nil
}

// SlowSearchInspection ES慢查询巡检上报程序入口
// @return error 错误
func SlowSearchInspection() (_err error) {
	// 初始化客户端
	client, err := ReadESClient()
	if err != nil {
		global.GVA_LOG.Error("初始化ES客户端失败", zap.Error(err))
		return err
	}

	// 查询慢查询日志，包括query查询和聚合查询（分桶聚合和获取每个分桶中一条样本数据）
	results, err := FetchSlowSearchAgg(client)
	if err != nil {
		global.GVA_LOG.Error("聚合查询失败", zap.Error(err))
		return err
	}

	// 输出结果
	for _, r := range results {
		//fmt.Printf("CheckSum: %s\n", r.SQLHash)
		//fmt.Printf("DocCount: %d\n", r.MySQLTotalExecutionCounts)
		//fmt.Printf("  时间: %s\n", r.SQLTime)
		//fmt.Printf("  品牌: %s\n", r.Brand)
		//fmt.Printf("  集群: %s\n", r.DBInstanceName)
		//fmt.Printf("  索引: %s\n", r.DBName)
		//fmt.Printf("  平均耗时: %.2f ms\n", r.AverageExecutionTime)
		//fmt.Printf("  最大耗时: %.2f ms\n", r.MaxExecutionTime)
		//fmt.Printf("  查询语句: %s\n\n", r.SQLText)

		// 从配置中获取中文名
		if cnName, ok := global.GVA_CONFIG.EsInspection.InstanceNameMap[r.DBInstanceName]; ok {
			r.DBInstanceName = cnName
		} else {
			defaultName := "未命名实例-" + r.DBInstanceName
			global.GVA_LOG.Warn("ES实例ID未配置映射，使用默认名称",
				zap.String("实例ID", r.DBInstanceName),
				zap.String("默认名称", defaultName))
			r.DBInstanceName = defaultName
		}

		// 将单条慢查询作为参数传给巡检上报接口
		err := ReportSlowSearch(r)
		if err != nil {
			global.GVA_LOG.Error("巡检上报失败",
				zap.String("CheckSum", r.SQLHash),
				zap.Error(err))
		} else {
			global.GVA_LOG.Info("巡检上报成功",
				zap.String("CheckSum", r.SQLHash))
		}
	}
	return nil
}
