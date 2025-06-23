package es

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/flipped-aurora/gin-vue-admin/server/global"
	esModel "github.com/flipped-aurora/gin-vue-admin/server/model/es"
	esReq "github.com/flipped-aurora/gin-vue-admin/server/model/es/request"
	"github.com/olivere/elastic/v7"
	"go.uber.org/zap"
	"reflect"
)

type AliEsSlowSearchLogService struct {
}

func (s *AliEsSlowSearchLogService) GetAliEsSearchSlowLogs(req esReq.GetAliEsSlowSearchList) (list interface{}, total int64, err error) {

	// 硬编码 Elasticsearch 配置
	esHost := "http://172.16.119.119:9200" // Elasticsearch 地址
	esUsername := ""                       // Elasticsearch 用户名
	esPassword := ""                       // Elasticsearch 密码
	indexName := "ali_es_slow_search_log"  // 索引名称

	// 创建 Elasticsearch 客户端
	client, err := elastic.NewClient(
		elastic.SetURL(esHost),
		elastic.SetBasicAuth(esUsername, esPassword),
		elastic.SetSniff(false),
	)
	if err != nil {
		global.GVA_LOG.Error("Failed to create Elasticsearch client", zap.Error(err))
		return nil, 0, err
	}
	// 打印连接成功信息
	global.GVA_LOG.Info("Elasticsearch client created successfully", zap.String("host", esHost))

	// 构建查询条件
	query := elastic.NewBoolQuery()

	// 添加查询条件
	// 校验开始时间和结束时间必须字段
	if req.StartTime == "" || req.EndTime == "" {
		global.GVA_LOG.Error("缺少必要的时间参数",
			zap.String("startTime", req.StartTime),
			zap.String("endTime", req.EndTime),
		)
		return nil, 0, errors.New("必须提供startTime和endTime")
	}

	// 3. 构建范围查询
	query = query.Must(elastic.NewRangeQuery("eventTime").
		Gte(req.StartTime).
		Lte(req.EndTime))

	// 创建QueryText全文检索查询
	if req.QueryText != "" {
		// 暂时用全文搜索，后面有问题再用wildcard查询（字段类型设置成keyword）
		query = query.Must(elastic.NewMatchQuery("queryText", req.QueryText))
	}

	// 创建耗时时间range查询
	durRangeQuery := elastic.NewRangeQuery("durationMs")

	// 检查并添加最小持续时间条件
	if req.MinDurationMs > 0 {
		durRangeQuery.Gte(req.MinDurationMs)
	}
	// 检查并添加最大持续时间条件
	if req.MaxDurationMs > 0 {
		durRangeQuery.Lte(req.MaxDurationMs)
	}
	if req.MinDurationMs > 0 || req.MaxDurationMs > 0 {
		query = query.Must(durRangeQuery)
	}

	if req.IndexName != "" {
		query = query.Must(elastic.NewTermQuery("indexName", req.IndexName))
	}
	if req.NodeIp != "" {
		query = query.Must(elastic.NewTermQuery("nodeIP", req.NodeIp))
	}
	if req.InstanceId != "" {
		query = query.Must(elastic.NewTermQuery("instanceId", req.InstanceId))
	}
	if req.Level != "" {
		query = query.Must(elastic.NewTermQuery("level", req.Level))
	}
	if req.SearchHits > 0 {
		query = query.Must(elastic.NewRangeQuery("searchHits").Gte(req.SearchHits))
	}
	if req.SearchType != "" {
		query = query.Must(elastic.NewTermQuery("searchType", req.SearchType))
	}
	if req.SlowSearchType != "" {
		query = query.Must(elastic.NewTermQuery("slowSearchType", req.SlowSearchType))
	}

	// 计算分页参数
	limit := req.PageSize
	offset := req.PageSize * (req.Page - 1)

	// 构建搜索源
	searchSource := elastic.NewSearchSource().
		Query(query).
		From(int(offset)).
		Size(int(limit))

	// 打印结构化DSL日志
	if source, err := searchSource.Source(); err != nil {
		global.GVA_LOG.Error("Failed to get search source",
			zap.Error(err),
			zap.String("index", indexName))
	} else {
		// 类型断言确保是map类型
		queryMap, ok := source.(map[string]interface{})
		if !ok {
			global.GVA_LOG.Error("Invalid query source type",
				zap.String("index", indexName),
				zap.Any("source_type", reflect.TypeOf(source)))
		} else {
			// 构建结构化日志字段
			logFields := []zap.Field{
				zap.String("index", indexName),
				zap.Int("from", offset),
				zap.Int("size", limit),
			}

			// 添加各查询组件（确保不为nil才添加）
			if query, exists := queryMap["query"]; exists && query != nil {
				logFields = append(logFields, zap.Any("query", query))
			}
			if sort, exists := queryMap["sort"]; exists && sort != nil {
				logFields = append(logFields, zap.Any("sort", sort))
			}
			if aggs, exists := queryMap["aggs"]; exists && aggs != nil {
				logFields = append(logFields, zap.Any("aggregations", aggs))
			}
			if highlight, exists := queryMap["highlight"]; exists && highlight != nil {
				logFields = append(logFields, zap.Any("highlight", highlight))
			}

			// 记录结构化日志
			global.GVA_LOG.Info("Elasticsearch Structured Query",
				logFields...)

			// 可选：同时记录原始完整DSL（DEBUG级别）
			if fullDSL, err := json.MarshalIndent(source, "", "  "); err == nil {
				global.GVA_LOG.Debug("Elasticsearch Full DSL",
					zap.String("index", indexName),
					zap.String("full_dsl", string(fullDSL)))
			}
		}
	}

	// 构建搜索请求
	searchService := client.Search().
		Index(indexName).
		SearchSource(searchSource)

	// 新增：打印原始请求体（直接获取并打印）
	if searchSource != nil {
		if body, err := searchSource.Source(); err == nil {
			if bodyJSON, err := json.Marshal(body); err == nil {
				global.GVA_LOG.Info("Elasticsearch Request Body",
					zap.String("index", indexName),
					zap.ByteString("body", bodyJSON))
			}
		}
	}

	// 在searchService.Do()之前添加
	src, _ := searchSource.Source()
	body, _ := json.Marshal(src)
	global.GVA_LOG.Info("Raw Request to ES",
		zap.ByteString("body", body),
		zap.String("index", indexName))

	// 执行搜索
	searchResult, err := searchService.Do(context.Background())
	if err != nil {
		global.GVA_LOG.Error("Failed to execute Elasticsearch search", zap.Error(err))
		return nil, 0, err
	}

	// 解析结果
	var slowSearchLogList []esModel.AliEsSlowSearchLog
	for _, hit := range searchResult.Hits.Hits {
		var log esModel.AliEsSlowSearchLog
		if err := json.Unmarshal(hit.Source, &log); err != nil {
			global.GVA_LOG.Error("Failed to unmarshal Elasticsearch hit", zap.Error(err))
			continue
		}
		slowSearchLogList = append(slowSearchLogList, log)
	}

	// 获取总命中数
	totalHits := searchResult.Hits.TotalHits
	if totalHits != nil {
		return slowSearchLogList, int64(totalHits.Value), nil
	}
	return slowSearchLogList, 0, nil
}
