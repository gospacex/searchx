// searchx/elasticsearch/examples: Elasticsearch 搜索示例
// 功能：Index、Search、Get、Delete、Bulk、HealthCheck
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/gospacex/configx"
	"github.com/gospacex/logx"
	_ "github.com/gospacex/logx/zap"
	"github.com/gospacex/searchx"
	_ "github.com/gospacex/searchx/elasticsearch"
)

func main() {
	ctx := context.Background()

	if err := logx.ReadConfig(); err != nil {
		log.Printf("logx.ReadConfig fallback: %v", err)
	}
	logger := logx.GetGlobal()
	logger.InfoContext(ctx, "searchx elasticsearch example started", logx.String("driver", "elasticsearch"))

	client, err := searchx.ReadAndCreate(configx.ConfigOptions{
		ConfigFile: "config.yaml",
		ConfigType: "yaml",
	})
	if err != nil {
		logger.Fatal("create searchx client failed", logx.Err(err))
	}
	defer client.Close()
	logger.InfoContext(ctx, "client driver", logx.String("driver", client.Driver()))

	// HealthCheck
	if err := client.HealthCheck(); err != nil {
		logger.Fatal("health check failed", logx.Err(err))
	}
	logger.InfoContext(ctx, "health check passed")

	indexName := "test-index"

	// Index 文档
	doc := searchx.Document{
		"id":    "doc-001",
		"title": "Elasticsearch Example",
		"body":  "Full-text search with Elasticsearch",
	}
	if err := client.Index(ctx, indexName, doc); err != nil {
		logger.Fatal("index failed", logx.Err(err))
	}
	logger.InfoContext(ctx, "document indexed", logx.String("id", "doc-001"))

	// Search
	result, err := client.Search(ctx, indexName, "Elasticsearch Example")
	if err != nil {
		logger.Fatal("search failed", logx.Err(err))
	}
	logger.InfoContext(ctx, "search completed", logx.Int("total", result.Total), logx.Int("hits", len(result.Hits)))

	// Get
	got, err := client.Get(ctx, indexName, "doc-001")
	if err != nil {
		logger.Fatal("get failed", logx.Err(err))
	}
	logger.InfoContext(ctx, "document retrieved", logx.String("title", got["title"].(string)))

	// Bulk
	if err := client.Bulk(ctx, indexName, []searchx.BulkOp{
		{Action: "index", ID: "doc-002", Doc: searchx.Document{"id": "doc-002", "title": "Bulk Test"}},
		{Action: "delete", ID: "doc-002", Doc: nil},
	}); err != nil {
		logger.ErrorContext(ctx, "bulk failed", logx.Err(err))
	} else {
		logger.InfoContext(ctx, "bulk operations completed")
	}

	// Delete
	if err := client.Delete(ctx, indexName, "doc-001"); err != nil {
		logger.Fatal("delete failed", logx.Err(err))
	}
	logger.InfoContext(ctx, "document deleted")

	fmt.Println("\n✅ All operations completed successfully.")
}
