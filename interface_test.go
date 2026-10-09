package searchx

import (
	"context"
	"strings"
	"sync"
	"testing"
)

// ===== Config.FillDefaults =====

func TestConfigFillDefaults(t *testing.T) {
	c := &Config{}
	c.FillDefaults()

	if len(c.Address) != 1 || c.Address[0] != "http://localhost:9200" {
		t.Errorf("Address = %v, want [http://localhost:9200]", c.Address)
	}
	if c.Name != "default" {
		t.Errorf("Name = %q, want %q", c.Name, "default")
	}
}

// TestConfigFillDefaultsKeepsExplicitValues 钉住「显式配置不被默认值覆盖」——
// 否则用户配了自己的 ES 地址会被静默改写成 localhost:9200，连到错误集群。
func TestConfigFillDefaultsKeepsExplicitValues(t *testing.T) {
	c := &Config{
		Address: []string{"http://es-a:9200", "http://es-b:9200"},
		Name:    "prod-search",
	}
	c.FillDefaults()

	if len(c.Address) != 2 || c.Address[0] != "http://es-a:9200" {
		t.Errorf("Address = %v, want 保留显式配置的两节点", c.Address)
	}
	if c.Name != "prod-search" {
		t.Errorf("Name = %q, want prod-search", c.Name)
	}
}

// ===== 工厂：未注册 driver 的错误必须可定位 =====

func TestNewUnregisteredDriver(t *testing.T) {
	_, err := New(&Config{Driver: "definitely-not-registered"})
	if err == nil {
		t.Fatal("未注册 driver 应返回 error，实际 nil")
	}
	if !strings.Contains(err.Error(), "not registered") {
		t.Errorf("错误信息应含 %q 以指引 import 驱动包，实际: %v", "not registered", err)
	}
}

// fakeSearchClient 仅用于验证工厂接线，不依赖任何外部中间件。
type fakeSearchClient struct{}

func (f *fakeSearchClient) Index(ctx context.Context, index string, doc Document) error {
	return nil
}
func (f *fakeSearchClient) Get(ctx context.Context, index, id string) (Document, error) {
	return nil, nil
}
func (f *fakeSearchClient) Delete(ctx context.Context, index, id string) error { return nil }
func (f *fakeSearchClient) Search(ctx context.Context, index, query string) (*QueryResult, error) {
	return &QueryResult{}, nil
}
func (f *fakeSearchClient) Bulk(ctx context.Context, index string, ops []BulkOp) error { return nil }
func (f *fakeSearchClient) HealthCheck() error                                         { return nil }
func (f *fakeSearchClient) Close() error                                               { return nil }
func (f *fakeSearchClient) Driver() string                                             { return "fake" }

func TestFactoryCreateAfterRegister(t *testing.T) {
	const drv = "test-factory-searchx"
	RegisterCreator(drv, func(c *Config) (SearchClient, error) { return &fakeSearchClient{}, nil })

	got, err := New(&Config{Driver: drv})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got == nil {
		t.Fatal("New 返回的 SearchClient 为 nil")
	}
}

// TestFactoryConcurrentAccess 钉住 creators map 的并发安全性（回归守卫）。
//
// searchx 的 registryMu 是 REVIEW-REPORT.md 第八节第 23 条补的（与 coordx/lockx
// 同源，但那两个被漏掉了、已另行修复）。本用例的作用是防止后续重构把锁删掉
// 而无人察觉 —— 若锁被移除，这里会在 -race 下触发
// "fatal error: concurrent map read and map write"。
//
// 读写两端都走受保护 API（RegisterCreator / New），不直接访问裸 map。
func TestFactoryConcurrentAccess(t *testing.T) {
	const n = 32
	var wg sync.WaitGroup
	wg.Add(n * 2)

	for i := 0; i < n; i++ {
		drv := "concurrent-searchx-" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		go func(d string) {
			defer wg.Done()
			RegisterCreator(d, func(c *Config) (SearchClient, error) { return &fakeSearchClient{}, nil })
		}(drv)

		// 命中与否都允许（注册与查找并发，顺序不定），只要不 panic / 不 race
		go func(d string) {
			defer wg.Done()
			_, _ = New(&Config{Driver: d})
		}(drv)
	}
	wg.Wait()
}
