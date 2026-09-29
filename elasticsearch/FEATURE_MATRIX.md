# Elasticsearch 驱动功能矩阵

> 代码依据：`elasticsearch.go`、`register.go`（searchx/elasticsearch）
> 更新日期：2026-09-09 | 驱动注册名：`elasticsearch` | 客户端库：`github.com/elastic/go-elasticsearch/v8`

## 1. 基础命令矩阵（SearchClient 接口）

| 接口方法 | 底层 API | 状态 | 行为说明 |
|---|---|---|---|
| `Index(ctx, index, doc)` | `ES.Index(index, body)` | ✅ 支持 | 索引文档，返回 `_id` |
| `Get(ctx, index, id)` | `ES.Get(index, id)` | ✅ 支持 | 获取单个文档 |
| `Delete(ctx, index, id)` | `ES.Delete(index, id)` | ✅ 支持 | 删除文档 |
| `Search(ctx, index, query)` | `ES.Search()` | ✅ 支持 | DSL 字符串查询，返回 `*QueryResult` |
| `Bulk(ctx, index, ops)` | `ES.Bulk()` | ✅ 支持 | 批量索引/删除操作 |
| `HealthCheck()` | `ES.Info()` | ✅ 支持 | 检查集群健康状态 |
| `Close()` | `http.Transport.CloseIdleConnections()` | ✅ 支持 | — |
| `Driver()` | 返回 `"elasticsearch"` | ✅ 支持 | — |

### OTel 追踪属性

| 属性 | 值 |
|---|---|
| span name | `search.elasticsearch.{OPERATION}` |
| `db.system` | `elasticsearch` |
| `db.operation` | `index` / `get` / `delete` / `search` / `bulk` |

## 2. 部署拓扑矩阵

| 模式 | 状态 | 说明 |
|---|---|---|
| 单机 Elasticsearch | ✅ 支持 | `cfg.Address[0]` 直连 |
| ES Cluster | ✅ 支持 | `cfg.Address` 多地址，客户端负载均衡 |
| TLS | ✅ 支持 | `cfg.TLS.Enabled` → `http.Transport.TLSConfig` |
| Basic Auth | ✅ 支持 | `cfg.Username`/`cfg.Password` |
| 索引别名 | ⚠️ 部分 | 驱动未封装别名切换 API，需业务侧直接调用 |
| 滚动索引（ILM） | ❌ 不支持 | 需业务侧实现 |

## 3. 配置字段生效状态（searchx.Config）

| 字段 | 状态 | 说明 |
|---|---|---|
| `Address` | ✅ 生效 | 逗号分隔多节点（Cluster 模式） |
| `Username` | ✅ 生效 | Basic Auth 用户名 |
| `Password` | ✅ 生效 | Basic Auth 密码 |
| `TLS.Enabled` | ✅ 生效 | — |
| `EnableTrace` | ⚠️ 部分 | span 始终上报 |
| `Name` / `Driver` | ✅ 生效 | — |

## 4. 扩展命令矩阵

| 命令 | 状态 | 说明 |
|---|---|---|
| Aggregation 聚合 | ✅ 支持 | Search 接口传入 DSL 即可，无类型安全封装 |
| Highlight 高亮 | ✅ 支持 | DSL 字符串传入 |
| Scroll 深度分页 | ✅ 支持 | DSL 字符串传入 |
| Reindex 跨索引复制 | ⚠️ 部分 | ES 原生 API，驱动未封装 |
| Index Template | ❌ 不支持 | 需业务侧直接调用 ES API |
| Index Lifecycle Management | ❌ 不支持 | — |
| DSL 字符串查询 | ✅ 支持 | 唯一查询方式 |
| Query DSL 类型安全 | ❌ 不支持 | 查询以字符串传入，无结构化 DSL 构建器 |

## 5. 企业级业务场景矩阵

| 业务场景 | 支持状态 | 实现方式与约束 |
|---|---|---|
| 全文检索 | ✅ 完整支持 | Search 接口覆盖 |
| 日志聚合分析 | ✅ 完整支持 | Bulk + Search |
| 商品/内容搜索 | ✅ 完整支持 | — |
| 结构化过滤查询 | ✅ 完整支持 | Bool query DSL |
| 地理位置搜索 | ⚠️ 部分 | geo_query DSL 可通过字符串传入 |
| 同义词搜索 | ⚠️ 部分 | synonym filter 需索引级别配置 |
| 实时搜索（近实时） | ✅ 支持 | refresh_interval=1s |
| 审计日志查询 | ✅ 完整支持 | — |
| 聚合分析（BI） | ✅ 完整支持 | Aggregation DSL |
| 实时告警匹配 | ✅ 完整支持 | Watcher/ILM 需业务侧配置 |
| 健康探针 | ✅ 支持 | ES Info 检查 |
| 链路追踪 | ✅ 支持 | `search.elasticsearch.*` span |

## 6. 已知限制与风险

1. **DSL 字符串查询无类型安全**：所有查询以 JSON 字符串传入，编译期无法校验 DSL 结构错误，必须在运行时捕获。
2. **无结构化 DSL 构建器**：无法在编译期校验 query/aggregation 结构，建议业务侧使用 JSON Schema 或测试用例保障正确性。
3. **聚合/高亮深度嵌套**：复杂聚合需手动构造 DSL，出错时排查成本高。
4. **Bulk 操作部分失败**：ES Bulk API 即使部分失败也返回 `status=200`，需业务侧解析 `items` 数组中的错误。
5. **近实时刷新延迟**：ES 默认 1s refresh，刚索引的文档立即可搜，但 scroll/deep pagination 需注意一致性窗口。

## 7. 快速接入示例

```go
import (
    "github.com/gospacex/searchx"
    _ "github.com/gospacex/searchx/elasticsearch" // 注册驱动
)

client, _ := searchx.New(&searchx.Config{
    Name: "default", Driver: "elasticsearch",
    Address: []string{"http://127.0.0.1:9200"},
    Username: "elastic", Password: "changeme",
})
```
