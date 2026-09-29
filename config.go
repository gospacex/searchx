// searchx/config.go — 统一配置集成（configx + ReadAndCreate）
package searchx

import (
	"fmt"
	"os"

	"github.com/gospacex/configx"
)

// StandaloneConfig 单实例搜索引擎配置（本地/viper，适用于单机部署）
type StandaloneConfig struct {
	Driver   string    `yaml:"driver" json:"driver" mapstructure:"driver"`
	Name     string    `yaml:"name" json:"name" mapstructure:"name"`
	Address  []string  `yaml:"address" json:"address" mapstructure:"address"`
	Username string    `yaml:"username" json:"username" mapstructure:"username"`
	Password string    `yaml:"password" json:"password" mapstructure:"password"`
	TLS      TLSConfig `yaml:"tls" json:"tls" mapstructure:"tls"`
	// EnableTrace 统一调用链开关（configx 配置键 enable_trace）
	EnableTrace bool `yaml:"enable_trace" json:"enable_trace" mapstructure:"enable_trace"`
}

// ClusterConfig 集群搜索引擎配置（Nacos/Apollo 远程配置中心，适用于多节点部署）
type ClusterConfig struct {
	Driver   string    `yaml:"driver" json:"driver" mapstructure:"driver"`
	Name     string    `yaml:"name" json:"name" mapstructure:"name"`
	Address  []string  `yaml:"address" json:"address" mapstructure:"address"`
	Username string    `yaml:"username" json:"username" mapstructure:"username"`
	Password string    `yaml:"password" json:"password" mapstructure:"password"`
	TLS      TLSConfig `yaml:"tls" json:"tls" mapstructure:"tls"`
	// Namespace 命名空间（多租户隔离）
	Namespace string `yaml:"namespace" json:"namespace" mapstructure:"namespace"`
	// WatchEnabled 是否监听配置变更
	WatchEnabled bool `yaml:"watch_enabled" json:"watch_enabled" mapstructure:"watch_enabled"`
	// EnableTrace 统一调用链开关（configx 配置键 enable_trace）
	EnableTrace bool `yaml:"enable_trace" json:"enable_trace" mapstructure:"enable_trace"`
}

// SearchSection 对应 configx.yaml 中的 searchx 配置节
type SearchSection struct {
	Standalone []*StandaloneConfig `yaml:"standalone"`
	Cluster    []*ClusterConfig    `yaml:"cluster"`
}

// ReadConfig 加载统一配置句柄（所有 x 系列 SDK 共享同一配置源）
func ReadConfig(opts ...configx.ConfigOptions) error {
	_, err := configx.ReadConfig(opts...)
	if err != nil {
		return err
	}
	return nil
}

// ReadAndCreate 从 configx 读取 searchx 配置并创建搜索引擎客户端
func ReadAndCreate(opts ...configx.ConfigOptions) (SearchClient, error) {
	c, err := configx.ReadConfig(opts...)
	if err != nil {
		return nil, fmt.Errorf("searchx: failed to read config: %w", err)
	}

	section := &SearchSection{}
	if err := c.Unmarshal("searchx", section); err != nil {
		return nil, fmt.Errorf("searchx: failed to unmarshal searchx section: %w", err)
	}

	var cfg *Config
	for _, cc := range section.Cluster {
		cfg = clusterToConfig(cc)
		break
	}
	if cfg == nil {
		for _, sc := range section.Standalone {
			cfg = standaloneToConfig(sc)
			break
		}
	}

	if cfg == nil {
		return nil, fmt.Errorf("searchx: no searchx instance configured")
	}

	return New(cfg)
}

// MustReadAndCreate 类似 ReadAndCreate，失败时 panic
func MustReadAndCreate(opts ...configx.ConfigOptions) SearchClient {
	client, err := ReadAndCreate(opts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "searchx: failed to initialize search client: %v\n", err)
		os.Exit(1)
	}
	return client
}

func standaloneToConfig(s *StandaloneConfig) *Config {
	return &Config{
		Driver:      s.Driver,
		Name:        s.Name,
		Address:     s.Address,
		Username:    s.Username,
		Password:    s.Password,
		TLS:         s.TLS,
		EnableTrace: s.EnableTrace,
	}
}

func clusterToConfig(c *ClusterConfig) *Config {
	return &Config{
		Driver:      c.Driver,
		Name:        c.Name,
		Address:     c.Address,
		Username:    c.Username,
		Password:    c.Password,
		TLS:         c.TLS,
		EnableTrace: c.EnableTrace,
	}
}
