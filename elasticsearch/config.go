// searchx/elasticsearch - Elasticsearch 搜索引擎驱动配置（Standalone + Cluster）
package elasticsearch

import (
	"github.com/gospacex/searchx"
)

// StandaloneConfig Elasticsearch 单实例配置
type StandaloneConfig struct {
	Driver   string            `yaml:"driver" json:"driver"`
	Name     string            `yaml:"name" json:"name"`
	Address  []string          `yaml:"address" json:"address"`
	Username string            `yaml:"username" json:"username"`
	Password string            `yaml:"password" json:"password"`
	TLS      searchx.TLSConfig `yaml:"tls" json:"tls"`
}

// ClusterConfig Elasticsearch 集群配置
type ClusterConfig struct {
	Driver    string            `yaml:"driver" json:"driver"`
	Name      string            `yaml:"name" json:"name"`
	Endpoints []string          `yaml:"endpoints" json:"endpoints"`
	Username  string            `yaml:"username" json:"username"`
	Password  string            `yaml:"password" json:"password"`
	TLS       searchx.TLSConfig `yaml:"tls" json:"tls"`
	// Cluster
	Namespace    string `yaml:"namespace" json:"namespace"`
	WatchEnabled bool   `yaml:"watch_enabled" json:"watch_enabled"`
	EnableTrace  bool   `yaml:"enable_trace" json:"enable_trace"`
}

// ToStandaloneConfig 转换为 searchx.Config
func (s *StandaloneConfig) ToStandaloneConfig(name string) *searchx.Config {
	n := name
	if s.Name != "" {
		n = s.Name
	}
	return &searchx.Config{
		Driver:   "elasticsearch",
		Name:     n,
		Address:  s.Address,
		Username: s.Username,
		Password: s.Password,
		TLS:      s.TLS,
	}
}

// ToClusterConfig 转换为 searchx.Config
func (c *ClusterConfig) ToClusterConfig(name string) *searchx.Config {
	n := name
	if c.Name != "" {
		n = c.Name
	}
	addrs := c.Endpoints
	return &searchx.Config{
		Driver:   "elasticsearch",
		Name:     n,
		Address:  addrs,
		Username: c.Username,
		Password: c.Password,
		TLS:      c.TLS,
	}
}
