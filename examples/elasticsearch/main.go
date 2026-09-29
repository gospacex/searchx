// searchx/examples/elasticsearch/main.go: Elasticsearch 搜索引擎示例
// 功能：索引、查询、删除、搜索、批量操作、驱动名、调用链追踪
package main

import (
	"context"
	"fmt"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"log"

	"github.com/gospacex/configx"
	"github.com/gospacex/logx"
	_ "github.com/gospacex/logx/zap"
	"github.com/gospacex/searchx"
	_ "github.com/gospacex/searchx/elasticsearch"
)

func initTracer() (*sdktrace.TracerProvider, error) {
	exporter, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint("http://localhost:14268/api/traces")))
	if err != nil {
		return nil, fmt.Errorf("jaeger exporter init failed: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.Default()),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)
	return tp, nil
}

func main() {
	ctx := context.Background()

	// 1. 初始化 logx
	if err := logx.ReadConfig(); err != nil {
		log.Printf("logx.ReadConfig fallback: %v", err)
	}
	logger := logx.GetGlobal()

	_, err := initTracer()
	if err != nil {
		logger.InfoContext(ctx, "tracer init skipped", logx.Err(err))
	}
	logger.InfoContext(ctx, "searchx elasticsearch example started", logx.String("driver", "elasticsearch"))

	// 2. 读取配置并创建客户端
	client, err := searchx.ReadAndCreate(configx.ConfigOptions{
		ConfigFile: "config.yaml",
		ConfigType: "yaml",
	})
	if err != nil {
		logger.Fatal("create search client failed", logx.Err(err))
	}
	defer client.Close()

	// 3. 健康检查
	if err := client.HealthCheck(); err != nil {
		logger.Fatal("health check failed", logx.Err(err))
	}
	logger.InfoContext(ctx, "health check passed")

	// 4. 获取驱动名称
	logger.InfoContext(ctx, "driver name", logx.String("driver", client.Driver()))

	indexName := "test-index"

	// 5. 索引文档（会自动创建索引）
	doc1 := searchx.Document{
		"title":      "Go 编程指南",
		"content":    "这是一本关于 Go 语言编程的指南",
		"created_at": "2024-01-01",
	}
	doc2 := searchx.Document{
		"title":      "Redis 使用技巧",
		"content":    "Redis 缓存和消息队列的最佳实践",
		"created_at": "2024-01-02",
	}
	logger.InfoContext(ctx, "indexing documents", logx.Int("count", 2))
	if err := client.Index(ctx, indexName, doc1); err != nil {
		logger.Fatal("index doc1 failed", logx.Err(err))
	}
	if err := client.Index(ctx, indexName, doc2); err != nil {
		logger.Fatal("index doc2 failed", logx.Err(err))
	}
	logger.InfoContext(ctx, "documents indexed")

	// 6. 获取文档
	logger.InfoContext(ctx, "getting document", logx.String("id", "doc1"))
	result, err := client.Get(ctx, indexName, "doc1")
	if err != nil {
		logger.Fatal("get document failed", logx.Err(err))
	}
	logger.InfoContext(ctx, "document retrieved", logx.String("title", result["title"].(string)))

	// 7. 搜索
	logger.InfoContext(ctx, "searching", logx.String("query", "Go 编程"))
	searchResult, err := client.Search(ctx, indexName, "Go 编程")
	if err != nil {
		logger.Fatal("search failed", logx.Err(err))
	}
	logger.InfoContext(ctx, "search completed", logx.Int("hits", searchResult.Total))

	// 8. 批量操作
	logger.InfoContext(ctx, "bulk indexing")
	bulkOps := []searchx.BulkOp{
		{Action: "index", ID: "doc3", Doc: searchx.Document{"title": "Elasticsearch 实战", "content": "ES 生产环境配置"}},
	}
	if err := client.Bulk(ctx, indexName, bulkOps); err != nil {
		logger.InfoContext(ctx, "bulk op note", logx.Err(err))
	}

	// 9. 删除文档
	logger.InfoContext(ctx, "deleting document", logx.String("id", "doc1"))
	if err := client.Delete(ctx, indexName, "doc1"); err != nil {
		logger.Fatal("delete failed", logx.Err(err))
	}
	logger.InfoContext(ctx, "document deleted")

	fmt.Println("\n✅ All operations completed successfully.")
}
