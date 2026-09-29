<div align="center">

# searchx

**统一搜索 SDK —— 一套 `SearchClient` 接口对接多种搜索引擎**

A unified search SDK for Go: one `SearchClient` interface, pluggable backends, configx-driven
configuration and OpenTelemetry tracing.

[![Go](https://img.shields.io/badge/Go-1.21-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](https://github.com/gospacex/searchx)
[![Release](https://img.shields.io/badge/release-v0.0.1-blue.svg)](https://github.com/gospacex/searchx/tags)

</div>

---

## 目录

- [1. 定位与设计边界](#1-定位与设计边界)
- [2. 核心特性](#2-核心特性)
- [3. 架构](#3-架构)
- [4. 快速开始](#4-快速开始)
- [5. 配置参考](#5-配置参考)
- [6. API 参考](#6-api-参考)
- [7. 后端矩阵](#7-后端矩阵)
- [8. 可观测性](#8-可观测性)
- [9. 示例与文档](#9-示例与文档)
- [10. 测试与质量](#10-测试与质量)
- [11. 版本与兼容性](#11-版本与兼容性)
- [12. 许可证](#12-许可证)

---

## 1. 定位与设计边界

searchx 是 hubx x 系列的**统一搜索抽象层**。业务代码只面向 `searchx.SearchClient` 接口，
具体搜索引擎由驱动实现，通过空导入（blank import）注册到驱动表，核心包不依赖任何驱动。

**解决的问题**

| 能力 | 说明 |
| :--- | :--- |
| 统一读写接口 | `Index` / `Get` / `Delete` / `Search` / `Bulk` 一套方法覆盖索引与检索 |
| 驱动可插拔 | 核心包零驱动依赖，`_ "github.com/gospacex/searchx/elasticsearch"` 即完成注册 |
| 双入口 | 代码构造 `searchx.New(&searchx.Config{...})`，或配置驱动 `searchx.ReadAndCreate(...)` |
| 单机 / 集群两种配置形态 | `searchx.standalone[]` 与 `searchx.cluster[]` 两个配置节 |
| 可观测 | 结构化日志走 logx，调用链 span 走 OpenTelemetry |

**不解决的问题（能力边界）**

- 不提供 Query DSL 构建器或类型安全查询 —— `Search` 只接受查询字符串，由驱动拼装请求体
- 不管理索引生命周期 —— 索引模板、别名切换、ILM、分片规划均需业务侧直接调用搜索引擎 API
- 不做聚合 / 高亮 / 分页的结构化封装 —— 这些能力通过查询字符串透传，返回值只做通用解析
- 不做业务级缓存、限流、鉴权 —— 只负责连接、读写与可观测

**与同系列 SDK 的关系**

| 组件 | 关系 |
| :--- | :--- |
| [configx](../configx) | 配置源。`ReadConfig` / `ReadAndCreate` / `MustReadAndCreate` 复用 `configx.ConfigOptions` |
| [logx](../logx) | 日志。驱动内使用 `logx.GetGlobal()` 输出结构化日志，空导入 `logx/zap` 注册 zap 后端 |
| OpenTelemetry | 调用链。驱动直接使用 `go.opentelemetry.io/otel` 创建 span，不依赖 otelx；接入宿主已有的 `TracerProvider` 即可 |
| dbx / cachex / mqx | 同为 x 系列，遵循「核心包 + 独立驱动 module + `init()` 自注册」的同一套组织方式 |

---

## 2. 核心特性

| 特性 | 说明 |
| :--- | :--- |
| 统一 `SearchClient` 接口 | 8 个方法固定语义，切换后端只需改配置中的 `driver` |
| 驱动独立 Go Module | `github.com/gospacex/searchx/elasticsearch` 单独发布，不引入即不进二进制 |
| `init()` 自注册 | 驱动在 `init()` 中调用 `searchx.RegisterCreator("elasticsearch", ...)`，无手动装配 |
| 单机 / 集群双配置节 | `standalone[]` 走本地配置，`cluster[]` 面向 Nacos / Apollo 等远程配置中心 |
| 批量写入 `Bulk` | 支持 `index` / `create` / `delete` / `update` 四类操作，逐条解析并记录失败项 |
| 建连重试 | 创建客户端失败时最多重试 3 次，间隔 100ms → 200ms 递增 |
| Basic Auth 与 TLS | `username` / `password` 走 Basic Auth；`tls.enabled` 打开底层传输的 TLS |
| 调用链埋点 | 开启 `EnableTrace` 后，每次读写产生 `db.elasticsearch.*` span 与 `db.system` 属性 |

---

## 3. 架构

```
业务代码
   │  searchx.SearchClient 接口
   ▼
┌──────────────────────────────────────────────────────────────┐
│  searchx 根包  github.com/gospacex/searchx                    │
│                                                              │
│  interface.go   SearchClient / Document / QueryResult /      │
│                 BulkOp / Config / TLSConfig                  │
│                 New() + RegisterCreator()  驱动注册表         │
│  config.go      ReadConfig / ReadAndCreate / MustReadAndCreate│
│                 SearchSection / StandaloneConfig / ClusterConfig│
│                 └── 读取 configx 统一配置源（searchx 配置节）    │
└──────────────────────────┬───────────────────────────────────┘
                           │  空导入触发 init() 注册 creator
                           ▼
┌──────────────────────────────────────────────────────────────┐
│  驱动 module  github.com/gospacex/searchx/elasticsearch       │
│                                                              │
│  register.go    searchx.RegisterCreator("elasticsearch", …)  │
│  elasticsearch.go  ClientImpl：Index / Get / Delete / Search │
│                    / Bulk / HealthCheck / Close / Driver     │
│                 go-elasticsearch/v8 + http.Transport         │
│                 logx 结构化日志 + otel.Tracer span            │
│  config.go      StandaloneConfig / ClusterConfig（驱动侧配置） │
└──────────────────────────┬───────────────────────────────────┘
                           │  HTTP
                           ▼
                    Elasticsearch 集群
```

分层原则：

1. **根包只定义抽象与工厂**，不 import 任何搜索引擎客户端；
2. **驱动是独立 module**，通过 `init()` 把创建函数注册进根包的 `creators` 表；
3. **配置统一由 configx 加载**，根包只负责把 `searchx` 配置节反序列化成 `searchx.Config`。

---

## 4. 快速开始

### 4.1 安装

```bash
go get github.com/gospacex/searchx@v0.0.1
go get github.com/gospacex/searchx/elasticsearch@v0.0.1
```

> `searchx` 与 `searchx/elasticsearch` 是两个独立 Go Module（各自由自己的 `go.mod` 定义），
> 驱动需单独引入。没有驱动时 `searchx.New` 会返回
> `driver "xxx" not registered` 错误。

本地已有 Elasticsearch 时可直接跳到 4.2；否则先起容器：

```bash
docker compose up -d     # 见第 9 节，ES 9200 / Kibana 5601
```

### 4.2 最小可用示例（10 行内跑通）

```go
package main

import (
	"context"
	"log"

	"github.com/gospacex/searchx"
	_ "github.com/gospacex/searchx/elasticsearch" // 空导入即完成驱动注册
)

func main() {
	ctx := context.Background()

	client, err := searchx.New(&searchx.Config{
		Driver:      "elasticsearch",
		Address:     []string{"http://localhost:9200"},
		EnableTrace: true, // 开启 OTel 调用链
	})
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	if err := client.Index(ctx, "docs", searchx.Document{"title": "Go 编程指南"}); err != nil {
		log.Fatal(err)
	}

	res, err := client.Search(ctx, "docs", "Go 编程")
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("total=%d hits=%d driver=%s", res.Total, len(res.Hits), client.Driver())
}
```

```bash
go run .
```

要点：

- 空白导入 `searchx/elasticsearch` 不能省略，否则驱动表为空；
- `Driver` 必须填 `elasticsearch`：驱动名未注册由 `searchx.New` 返回错误，与驱动期望值不符则由驱动返回错误；
- `Address` 为空时由 `Config.FillDefaults()` 兜底为 `http://localhost:9200`；
- `searchx.Document` 就是 `map[string]interface{}`，字段随业务自定义。

### 4.3 进阶用法（配置文件方式）

配置文件（`config.yaml`）：

```yaml
searchx:
  standalone:
    - driver: elasticsearch
      name: default
      address:
        - "http://localhost:9200"
      username: ""
      password: ""
      tls:
        enabled: false
```

代码：

```go
package main

import (
	"context"
	"log"

	"github.com/gospacex/configx"
	"github.com/gospacex/searchx"
	_ "github.com/gospacex/searchx/elasticsearch"
)

func main() {
	ctx := context.Background()

	client, err := searchx.ReadAndCreate(configx.ConfigOptions{
		ConfigFile: "config.yaml",
		ConfigType: "yaml",
	})
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	if err := client.HealthCheck(); err != nil {
		log.Fatal(err)
	}
	if err := client.Index(ctx, "docs", searchx.Document{"title": "Go 编程指南"}); err != nil {
		log.Fatal(err)
	}
	log.Printf("driver=%s", client.Driver())
}
```

启动即失败的场景用 `searchx.MustReadAndCreate(...)`：它与 `ReadAndCreate` 签名一致，
失败时把错误打印到 stderr 并以退出码 1 结束进程，适合「配置错误就不要起来」的部署方式。

> **注意**：`ReadAndCreate` / `MustReadAndCreate` 构造出的 `searchx.Config` 不设置
> `EnableTrace`（YAML 的 `searchx.standalone[]` / `searchx.cluster[]` 里也没有对应键），
> 此时调用 `Get` / `Search` / `Delete` 会因 span 未创建而 panic。
> 需要这三个方法时，请改用 4.2 的 `searchx.New` 并显式设置 `EnableTrace: true`。
> `Index` / `Bulk` / `HealthCheck` 不受影响。

---

## 5. 配置参考

### 5.1 完整 YAML 示例

```yaml
searchx:
  # 集群配置节：面向 Nacos / Apollo 等远程配置中心，多节点部署
  # ReadAndCreate 优先读取本节，命中第一条即使用
  cluster:
    - driver: elasticsearch
      name: default
      address:
        - "http://es-1:9200"
        - "http://es-2:9200"
        - "http://es-3:9200"
      username: ""
      password: ""
      tls:
        enabled: false
      namespace: ""
      watch_enabled: false

  # 单机配置节：本地 / viper 配置，cluster 为空时才生效
  standalone:
    - driver: elasticsearch
      name: default
      address:
        - "http://localhost:9200"
      username: ""
      password: ""
      tls:
        enabled: false
```

反序列化目标是 `searchx.SearchSection`（`config.go`），顶层键固定为 `searchx`。

### 5.2 `searchx.standalone[]` / `searchx.cluster[]` 字段表

| 字段 | 类型 | 必填 | 默认值 | 说明 |
| :--- | :--- | :---: | :--- | :--- |
| `driver` | string | 是 | — | 驱动名，当前只能填 `elasticsearch` |
| `name` | string | 否 | `default` | 实例名；`Config.FillDefaults()` 兜底为 `default`，ES 驱动不参与寻址 |
| `address` | []string | 否 | `["http://localhost:9200"]` | 节点地址列表；集群模式填多个节点，驱动整体交给 go-elasticsearch 分发 |
| `username` | string | 否 | 空 | Basic Auth 用户名 |
| `password` | string | 否 | 空 | Basic Auth 密码 |
| `tls.enabled` | bool | 否 | `false` | 为 true 时底层 `http.Transport` 启用 TLS（`InsecureSkipVerify: true`，跳过证书校验，仅适合内网自签场景） |
| `namespace` | string | 否 | 空 | **仅 `cluster[]`**。命名空间（多租户隔离），当前版本解析后不参与客户端构造 |
| `watch_enabled` | bool | 否 | `false` | **仅 `cluster[]`**。配置变更监听开关；本地 YAML（viper）路径下不生效，且转换为 `searchx.Config` 时也未使用该字段 |

### 5.3 单机 / 集群两种模式的差异

| 维度 | 单机（`standalone[]`） | 集群（`cluster[]`） |
| :--- | :--- | :--- |
| 结构体 | `searchx.StandaloneConfig` | `searchx.ClusterConfig` |
| 节点字段 | `address` | `address` |
| 独有字段 | 无 | `namespace`、`watch_enabled` |
| 生效顺序 | `cluster[]` 为空时才取第一条 | `ReadAndCreate` 先遍历 `cluster[]`，取第一条即返回 |
| 典型地址 | 1 个节点 | 多个节点（`go-elasticsearch` 在节点间分发请求） |
| 适用场景 | 本地开发、单机部署 | 多节点生产部署、远程配置中心统一下发 |

驱动包 `elasticsearch` 另有一组自己的配置结构（见 6.3），字段差异需要注意：
其 `ClusterConfig` 使用 **`endpoints`** 而不是 `address`，并额外提供 `enable_trace` 键。

### 5.4 `searchx.Config` 字段（代码构造方式）

`searchx.Config` 本身**没有 yaml / json tag**，只能在代码里构造；YAML 配置经由
`StandaloneConfig` / `ClusterConfig` 转换而来。

| 字段 | 类型 | 必填 | 默认值 | 说明 |
| :--- | :--- | :---: | :--- | :--- |
| `Driver` | string | 是 | — | 驱动名，`New` 据此查驱动表 |
| `Name` | string | 否 | `default` | 实例名，`FillDefaults()` 填充 |
| `Address` | []string | 否 | `["http://localhost:9200"]` | 节点地址，`FillDefaults()` 填充 |
| `Username` | string | 否 | 空 | Basic Auth 用户名 |
| `Password` | string | 否 | 空 | Basic Auth 密码 |
| `TLS` | `TLSConfig` | 否 | `TLSConfig{Enabled: false}` | 内嵌结构，只有 `Enabled bool` |
| `EnableTrace` | bool | 否 | `false` | 是否为每次操作创建 OTel span（ES 驱动的 `Get` / `Search` / `Delete` 依赖该开关，见 8.2） |

---

## 6. API 参考

### 6.1 核心接口 `searchx.SearchClient`

```go
type SearchClient interface {
    Index(ctx context.Context, index string, doc Document) error
    Get(ctx context.Context, index, id string) (Document, error)
    Delete(ctx context.Context, index, id string) error
    Search(ctx context.Context, index, query string) (*QueryResult, error)
    Bulk(ctx context.Context, index string, ops []BulkOp) error
    HealthCheck() error
    Close() error
    Driver() string
}
```

| 方法 | 签名 | 说明 |
| :--- | :--- | :--- |
| `Index` | `(ctx, index string, doc Document) error` | 写入（或自动创建）文档，不指定 id |
| `Get` | `(ctx, index, id string) (Document, error)` | 按 id 取单个文档，返回原始文档 map |
| `Delete` | `(ctx, index, id string) error` | 按 id 删除文档 |
| `Search` | `(ctx, index, query string) (*QueryResult, error)` | 查询；`query` 为空表示全量匹配，否则按全文匹配查询 |
| `Bulk` | `(ctx, index string, ops []BulkOp) error` | 批量写入 / 删除 / 更新；`ops` 为空直接返回 nil |
| `HealthCheck` | `() error` | 探测后端可用性（ES 驱动走集群健康接口） |
| `Close` | `() error` | 释放连接（关闭底层 HTTP transport 的空闲连接） |
| `Driver` | `() string` | 返回驱动名，ES 驱动固定 `"elasticsearch"` |

### 6.2 类型与工厂函数（根包 `github.com/gospacex/searchx`）

| 符号 | 定义 | 说明 |
| :--- | :--- | :--- |
| `Document` | `map[string]interface{}` | 文档，字段由业务自定义 |
| `QueryResult` | `struct { Total int; Hits []Document; Score float64; Error error }` | 查询结果；ES 驱动填充 `Total` 与 `Hits` |
| `BulkOp` | `struct { Action, ID string; Doc Document }` | 批量操作项，`Action` 取 `index` / `create` / `delete` / `update` |
| `Config` | 见 5.4 | 代码构造用的连接配置 |
| `TLSConfig` | `struct { Enabled bool }` | TLS 开关 |
| `New(cfg *Config) (SearchClient, error)` | 工厂 | 先 `FillDefaults()`，再按 `cfg.Driver` 查表创建 |
| `RegisterCreator(name string, creator func(*Config) (SearchClient, error))` | 工厂 | 注册驱动创建函数，由驱动的 `init()` 调用 |
| `(*Config).FillDefaults()` | 方法 | `Address` 空 → `["http://localhost:9200"]`；`Name` 空 → `default` |
| `ReadConfig(opts ...configx.ConfigOptions) error` | 配置 | 只加载 configx 全局配置句柄 |
| `ReadAndCreate(opts ...configx.ConfigOptions) (SearchClient, error)` | 配置 | 读取 `searchx` 配置节并创建客户端；`cluster[]` 优先于 `standalone[]` |
| `MustReadAndCreate(opts ...configx.ConfigOptions) SearchClient` | 配置 | 同上，失败时打印错误并 `os.Exit(1)` |
| `SearchSection` | 配置 | `Standalone []*StandaloneConfig` + `Cluster []*ClusterConfig` |
| `StandaloneConfig` | 配置 | 单机配置，字段见 5.2 |
| `ClusterConfig` | 配置 | 集群配置，字段见 5.2 |

### 6.3 驱动包 `github.com/gospacex/searchx/elasticsearch`

| 符号 | 签名 / 定义 | 说明 |
| :--- | :--- | :--- |
| `ClientImpl` | `struct{ ... }` | 实现 `searchx.SearchClient` 的 ES 客户端 |
| `New(cfg *searchx.Config) (*ClientImpl, error)` | 工厂 | 校验 `cfg.Driver == "elasticsearch"`，创建失败自动重试 3 次 |
| `(*ClientImpl).Index / Get / Delete / Search / Bulk / HealthCheck / Close / Driver` | 接口实现 | 逐条见 6.1 |
| `StandaloneConfig` | `driver` `name` `address` `username` `password` `tls` | 驱动侧单机配置 |
| `(*StandaloneConfig).ToStandaloneConfig(name string) *searchx.Config` | 转换 | 结构体 → `searchx.Config`，`Name` 为空时用入参 `name` |
| `ClusterConfig` | `driver` `name` **`endpoints`** `username` `password` `tls` `namespace` `watch_enabled` `enable_trace` | 驱动侧集群配置 |
| `(*ClusterConfig).ToClusterConfig(name string) *searchx.Config` | 转换 | `endpoints` → `searchx.Config.Address` |

使用驱动包直接构造时注意：`elasticsearch.New` **不会**调用 `FillDefaults()`，
`cfg.Address` 必须非空，否则会因取 `cfg.Address[0]` 而越界。推荐统一走 `searchx.New`。

---

## 7. 后端矩阵

| 后端 | 模块路径 | 底层客户端 | 状态 |
| :--- | :--- | :--- | :--- |
| Elasticsearch | `github.com/gospacex/searchx/elasticsearch` | `github.com/elastic/go-elasticsearch/v8`（v8.15.0） | ✅ 已提供 |

> searchx 的设计目标是多后端：接口（`SearchClient`）、工厂（`RegisterCreator`）、
> 配置（`driver` 字段）三者都与具体后端解耦，新增后端只需实现接口、在 `init()` 里注册，
> 并按独立 Go Module 发布。
>
> **roadmap**：目前仓库中仅实现 Elasticsearch 一个后端，其他搜索引擎（含 OpenSearch 及其他
> 全文检索服务）尚未实现，接入方式与 `elasticsearch` 子包完全一致。

驱动能力细节（支持的操作、部署拓扑、已知限制）见
[elasticsearch/FEATURE_MATRIX.md](elasticsearch/FEATURE_MATRIX.md)。

---

## 8. 可观测性

### 8.1 日志

驱动通过 `logx.GetGlobal()` 输出结构化日志，关键事件包括：

| 事件 | 日志内容 |
| :--- | :--- |
| 客户端创建 | `searchx/elasticsearch client created`（含 `addr`） |
| 索引 / 删除 | `searchx/es index done`、`searchx/es delete done`（含 `index`、`id`） |
| 查询 | `searchx/es search done`（含 `index`、`total`） |
| 批量 | `searchx/es bulk done` / `searchx/es bulk item error`（含 `count`、失败项） |

根包 `interface.go` 与驱动包均空导入 `github.com/gospacex/logx/zap`，
未显式初始化时会使用 logx 的默认后端；调用 `logx.ReadConfig()` 可按自身配置接管日志。

### 8.2 调用链（OpenTelemetry）

当 `searchx.Config.EnableTrace == true` 时，驱动在 `otel.Tracer("github.com/gospacex/searchx/elasticsearch")`
上创建 span：

| 操作 | span 名 | 附加属性 |
| :--- | :--- | :--- |
| `Index` | `db.elasticsearch.index` | `db.system=elasticsearch`、`db.elasticsearch.index` |
| `Get` | `db.elasticsearch.get` | `db.elasticsearch.index`、`db.elasticsearch.id` |
| `Delete` | `db.elasticsearch.delete` | `db.elasticsearch.index`、`db.elasticsearch.id` |
| `Search` | `db.elasticsearch.search` | `db.elasticsearch.index` |
| `Bulk` | `db.elasticsearch.bulk` | `db.system`、`db.elasticsearch.index`、`db.elasticsearch.bulk.operation_count` |

span 挂到调用方传入的 `ctx` 上，宿主应用已有的 `otel.SetTracerProvider(...)` 即可统一收集
（jaeger / OTLP 导出由宿主决定，searchx 不内置导出器）。

**开关约束**：`EnableTrace` 只能通过 `searchx.New` 显式设置 —— YAML 配置节里没有对应键，
`ReadAndCreate` 构造出的配置恒为 `false`；而 ES 驱动的 `Get` / `Search` / `Delete`
会在该开关为 `false` 时因 span 未创建而 panic（`Index` / `Bulk` / `HealthCheck` 不受影响）。
因此：**只要使用这三个方法，就必须保持 `EnableTrace: true`**。

### 8.3 指标（Metrics）

当前版本未内置 Prometheus 指标或 `/metrics` 端点，需要指标时请在业务侧包装 `SearchClient`
或基于底层 `go-elasticsearch` 客户端自行埋点。

---

## 9. 示例与文档

### 9.1 目录结构

```text
searchx/
├── interface.go            # SearchClient 接口、类型、New / RegisterCreator
├── config.go               # ReadConfig / ReadAndCreate / MustReadAndCreate、配置结构
├── go.mod                  # module github.com/gospacex/searchx
├── docker-compose.yml      # 本地 Elasticsearch + Kibana
├── verify.sh               # 一键验证脚本
├── examples/
│   └── elasticsearch/
│       ├── main.go         # 端到端示例：索引 / 获取 / 搜索 / 批量 / 删除
│       └── config.yaml     # 示例配置（searchx.standalone[]）
└── elasticsearch/          # 独立 Go Module：github.com/gospacex/searchx/elasticsearch
    ├── elasticsearch.go    # ClientImpl 实现
    ├── config.go           # 驱动侧 StandaloneConfig / ClusterConfig
    ├── register.go         # init() 注册驱动
    ├── FEATURE_MATRIX.md   # 驱动能力矩阵
    └── examples/main.go    # 驱动模块内示例
```

### 9.2 本地启动

```bash
docker compose up -d
```

`docker-compose.yml` 提供的服务与端口：

| 服务 | 容器名 | 端口 | 说明 |
| :--- | :--- | :--- | :--- |
| `elasticsearch` | `hubx-elasticsearch` | `9200`（HTTP）、`9300`（transport） | 单节点（`discovery.type=single-node`），关闭 xpack security，带健康检查 |
| `kibana` | `hubx-kibana` | `5601` | 依赖 ES 健康后启动，`ELASTICSEARCH_HOSTS=http://elasticsearch:9200` |

网络：`hubx-searchx-net`（bridge）；数据卷：`./data/elasticsearch`。

### 9.3 一键验证

```bash
./verify.sh                  # 默认驱动 elasticsearch
./verify.sh elasticsearch    # 显式指定
```

脚本流程：`docker compose up -d` → 轮询 `hubx-elasticsearch` 健康状态（最多 30 次 × 2s）→
`cd examples/elasticsearch && go run .` → `docker compose down`。

### 9.4 相关文档

- [elasticsearch/FEATURE_MATRIX.md](elasticsearch/FEATURE_MATRIX.md)：ES 驱动能力矩阵与已知限制

---

## 10. 测试与质量

```bash
# 编译（根包 + 驱动包 + 示例）
go build ./...

# 静态检查
go vet ./...

# 格式化
gofmt -l .

# 单元测试（当前仓库尚未提供 *_test.go）
go test ./...

# 端到端：依赖 docker compose，跑通 examples/elasticsearch
./verify.sh elasticsearch
```

当前质量校验以**编译 + 静态检查 + 示例实跑**为主；`verify.sh` 会真实起容器并执行示例，
是验证「配置 → 连接 → 写入 → 查询 → 删除」完整链路的入口。

---

## 11. 版本与兼容性

| 项 | 值 |
| :--- | :--- |
| Go 版本 | `go 1.21`（取自 `go.mod`，根包与驱动包一致） |
| 当前 tag | `v0.0.1` |
| 根包 module | `github.com/gospacex/searchx` |
| 驱动 module | `github.com/gospacex/searchx/elasticsearch` |
| 驱动客户端 | `github.com/elastic/go-elasticsearch/v8 v8.15.0` |
| 配置框架 | `github.com/gospacex/configx` |
| 日志 | `github.com/gospacex/logx`（zap 后端） |
| 调用链 | `go.opentelemetry.io/otel v1.29.0` |

驱动为独立 module，升级驱动不必升级根包；反之，根包接口变更（`SearchClient` 方法增删）
属于破坏性变更，需同步升主版本号。

---

## 12. 许可证

MIT License。
