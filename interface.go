// searchx: 搜索引擎统一接口，支持 elasticsearch 驱动
package searchx

import (
	"context"
	"fmt"
	"sync"

	_ "github.com/gospacex/logx/zap"
	"github.com/gospacex/traceflag"
)

// Document 搜索文档
type Document map[string]interface{}

// QueryResult 查询结果
type QueryResult struct {
	Total int
	Hits  []Document
	Score float64
	Error error
}

// SearchClient 搜索引擎客户端接口
type SearchClient interface {
	// Index 索引文档
	Index(ctx context.Context, index string, doc Document) error
	// Get 获取单个文档
	Get(ctx context.Context, index, id string) (Document, error)
	// Delete 删除文档
	Delete(ctx context.Context, index, id string) error
	// Search 搜索文档
	Search(ctx context.Context, index, query string) (*QueryResult, error)
	// Bulk 批量操作
	Bulk(ctx context.Context, index string, ops []BulkOp) error
	// HealthCheck 健康检查
	HealthCheck() error
	// Close 关闭连接
	Close() error
	// Driver 返回驱动名称
	Driver() string
}

// BulkOp 批量操作类型
type BulkOp struct {
	Action string // index, delete, update
	ID     string
	Doc    Document
}

// Config 搜索引擎配置
//
// 为什么逐字段都带三套 tag：本结构体是 configx 三个通道（yaml 本地文件 /
// json 远程配置中心 / mapstructure viper）的共同落点。缺 mapstructure tag 时
// viper 只能按**字段名**匹配，`enable_trace` / `legacy_cluster` 这类蛇形键解码
// 不进来 —— 不报错，只是永远取零值，表现为「明明写了开关却没生效」。
// 嵌套的 TLSConfig 同理（tls.enabled 需要两级 tag 都齐）。
type Config struct {
	Driver      string    `yaml:"driver" json:"driver" mapstructure:"driver"`
	Name        string    `yaml:"name" json:"name" mapstructure:"name"`
	Address     []string  `yaml:"address" json:"address" mapstructure:"address"`
	Username    string    `yaml:"username" json:"username" mapstructure:"username"`
	Password    string    `yaml:"password" json:"password" mapstructure:"password"`
	TLS         TLSConfig `yaml:"tls" json:"tls" mapstructure:"tls"`
	EnableTrace bool      `yaml:"enable_trace" json:"enable_trace" mapstructure:"enable_trace"`
	// LegacyCluster 兼容 Elasticsearch 7.14 之前的集群。
	//
	// 背景：官方 v8 客户端在每次 2xx 响应后无条件校验 X-Elastic-Product 响应头，
	// 而 7.13 及更早的集群不返回该头，结果是任何请求都报
	// "the client noticed that the server is not Elasticsearch and we do not support this unknown product"。
	// 打开本开关后，驱动会在响应上补出该头，让校验通过。
	//
	// 默认关闭：只影响旧集群的连通性判定，不改变任何其它行为，也不放宽安全校验
	// （TLS 仍按 cfg.TLS 处理）。升级到 7.14+ 后应当摘掉这个开关。
	LegacyCluster bool `yaml:"legacy_cluster" json:"legacy_cluster" mapstructure:"legacy_cluster"`
}

// TLSConfig TLS 配置
type TLSConfig struct {
	Enabled bool `yaml:"enabled" json:"enabled" mapstructure:"enabled"`
}

// FillDefaults 填充默认值
func (c *Config) FillDefaults() {
	if len(c.Address) == 0 {
		c.Address = []string{"http://localhost:9200"}
	}
	if c.Name == "" {
		c.Name = "default"
	}
}

// ===== Factory =====

var (
	registryMu sync.RWMutex
	creators   = map[string]func(*Config) (SearchClient, error){}
)

// RegisterCreator 注册搜索驱动创建函数
func RegisterCreator(name string, creator func(*Config) (SearchClient, error)) {
	registryMu.Lock()
	defer registryMu.Unlock()
	creators[name] = creator
}

// New 创建搜索引擎客户端
func New(cfg *Config) (SearchClient, error) {
	cfg.FillDefaults()
	// 接入 hubx 统一调用链开关：组件自身配置为主开关，
	// 全局总闸 traceflag.SetEnabled(false) 可一键熔断 searchx。
	cfg.EnableTrace = traceflag.Apply("searchx", cfg.EnableTrace)

	registryMu.RLock()
	creator, ok := creators[cfg.Driver]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("searchx: driver %q not registered; make sure to import the driver package", cfg.Driver)
	}

	client, err := creator(cfg)
	if err != nil {
		return nil, fmt.Errorf("searchx: create client %q: %w", cfg.Driver, err)
	}
	return client, nil
}

// ReadConfig 加载统一配置句柄（所有 x 系列 SDK 共享同一配置源）
// 详见 searchx/config.go 中的 ReadConfig 与 ReadAndCreate
