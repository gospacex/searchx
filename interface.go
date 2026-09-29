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
type Config struct {
	Driver      string
	Name        string
	Address     []string
	Username    string
	Password    string
	TLS         TLSConfig
	EnableTrace bool
}

// TLSConfig TLS 配置
type TLSConfig struct {
	Enabled bool
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
