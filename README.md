# Ops-Data-Govern 运维数据治理平台

## 项目概述

Ops-Data-Govern 是一款基于优秀开源框架 [gin-vue-admin](https://github.com/flipped-aurora/gin-vue-admin/) 构建的企业级运维数据治理平台，专注于解决混合环境下的多源异构数据治理难题。项目充分利用Go语言的高并发特性和Vue.js的前端优势，实现了对ElasticSearch、MySQL等数据源的数据治理，未来将扩展支持Redis、MongoDB、Kafka等主流中间件。

## 技术架构

### 核心框架
- **后端框架**：基于 gin-vue-admin 的 Golang 实现（Gin + Gorm）
- **前端框架**：Vue3 + Element Plus + TypeScript
- **并发模型**：Goroutine + Channel 实现生产者消费者模式
- **任务调度**：实现gin-vue-admin 的定时任务模块

## 核心功能

### 1. ElasticSearch慢查询
| 功能模块       | 技术实现                      | 性能指标               |
|----------------|-----------------------------|-----------------------|
| 阿里云ElasticSearch慢查询      | 多线程调用阿里云API接口 + Bulk批量更新       | 根据参数化配置batch-size，实现秒级同步插入       |