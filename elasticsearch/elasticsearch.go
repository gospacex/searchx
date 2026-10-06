// searchx/elasticsearch: Elasticsearch 搜索引擎驱动
package elasticsearch

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	es "github.com/elastic/go-elasticsearch/v8"
	"github.com/gospacex/logx"
	_ "github.com/gospacex/logx/zap"
	"github.com/gospacex/searchx"
	"github.com/gospacex/traceflag"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// ClientImpl Elasticsearch 客户端
type ClientImpl struct {
	es     *es.Client
	cfg    *searchx.Config
	tracer *traceflag.Tracer
	logger logx.Logger
	ht     *http.Transport // underlying HTTP transport for CloseIdleConnections
}

// New 创建 Elasticsearch 客户端
func New(cfg *searchx.Config) (*ClientImpl, error) {
	if cfg.Driver != "elasticsearch" {
		return nil, fmt.Errorf("elasticsearch: expected driver 'elasticsearch', got %q", cfg.Driver)
	}
	addr := cfg.Address[0]
	if addr == "" {
		addr = "http://localhost:9200"
	}

	ht := &http.Transport{}
	esCfg := es.Config{
		Addresses: cfg.Address,
		Username:  cfg.Username,
		Password:  cfg.Password,
		Transport: ht,
	}
	if cfg.TLS.Enabled {
		ht.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	// 旧集群（<7.14）不返回 X-Elastic-Product，v8 客户端会拒绝每一个 2xx 响应。
	// 显式开启 legacy_cluster 时才包一层补齐该头；默认路径零开销、行为不变。
	if cfg.LegacyCluster {
		esCfg.Transport = &productHeaderTransport{base: ht}
	}

	var client *es.Client
	if err := withRetry(func() error {
		var err2 error
		client, err2 = es.NewClient(esCfg)
		return err2
	}, 3); err != nil {
		return nil, fmt.Errorf("searchx/elasticsearch: create client: %w", err)
	}
	ctxLog := context.Background()
	logx.GetGlobal().InfoContext(ctxLog, "searchx/elasticsearch client created", logx.String("addr", addr))

	return &ClientImpl{
		es:     client,
		cfg:    cfg,
		tracer: traceflag.NewTracer(cfg.EnableTrace, "github.com/gospacex/searchx/elasticsearch", "searchx"),
		logger: logx.GetGlobal(),
		ht:     ht,
	}, nil
}

// productHeaderTransport 在响应上补出 X-Elastic-Product 头，供官方 v8 客户端的
// 产品校验通过；仅当 cfg.LegacyCluster 打开时挂载。
//
// 为什么做在 RoundTripper 这一层：客户端的校验发生在 Transport.Perform 返回之后、
// 业务读到响应之前，改响应头是唯一既不动 SDK 源码、又不影响请求语义的注入点。
// 只补 2xx 且不覆盖已有的值 —— 真集群自己带了头时以服务端为准。
type productHeaderTransport struct {
	base http.RoundTripper
}

// productHeader 是官方客户端校验时期望看到的固定值（genuineCheckHeader）。
const productHeader = "X-Elastic-Product"

func (t *productHeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	res, err := t.base.RoundTrip(req)
	if err != nil || res == nil {
		return res, err
	}
	if res.StatusCode >= 200 && res.StatusCode < 300 && res.Header.Get(productHeader) == "" {
		res.Header.Set(productHeader, "Elasticsearch")
	}
	return res, nil
}

func (c *ClientImpl) Index(ctx context.Context, index string, doc searchx.Document) error {
	var span trace.Span
	if c.cfg != nil && c.cfg.EnableTrace {
		ctx, span = c.tracer.Start(ctx, "db.elasticsearch.index")
		defer span.End()
	}
	if span != nil {
		span.SetAttributes(
			attribute.String("db.system", "elasticsearch"),
			attribute.String("db.elasticsearch.index", index),
		)
	}
	body, _ := json.Marshal(doc)
	req, err := c.es.Index(index, bytes.NewReader(body))
	if err != nil {
		if span != nil {
			span.RecordError(err)
		}
		c.logger.ErrorContext(ctx, "searchx/es index failed", logx.Err(err), logx.String("index", index))
		return fmt.Errorf("searchx/es index: %w", err)
	}
	_ = req
	c.logger.InfoContext(ctx, "searchx/es index done", logx.String("index", index))
	return nil
}

func (c *ClientImpl) Get(ctx context.Context, index, id string) (searchx.Document, error) {
	var span trace.Span
	if c.cfg != nil && c.cfg.EnableTrace {
		ctx, span = c.tracer.Start(ctx, "db.elasticsearch.get")
		defer span.End()
	}
	if span != nil {
		span.SetAttributes(
			attribute.String("db.elasticsearch.index", index),
			attribute.String("db.elasticsearch.id", id),
		)
	}
	res, err := c.es.Get(index, id)
	if err != nil {
		if span != nil {
			span.RecordError(err)
		}
		return nil, fmt.Errorf("searchx/es get: %w", err)
	}
	defer res.Body.Close()
	// 只取 _source：ES 的 Get 响应是一层信封（_index/_id/found/_source…），
	// 直接解码整包会把信封字段当成业务字段返回给调用方。
	var envelope struct {
		Found  bool             `json:"found"`
		Source searchx.Document `json:"_source"`
	}
	if err := json.NewDecoder(res.Body).Decode(&envelope); err != nil {
		return nil, err
	}
	if !envelope.Found {
		return nil, fmt.Errorf("searchx/es get: document %q not found in index %q", id, index)
	}
	return envelope.Source, nil
}

func (c *ClientImpl) Delete(ctx context.Context, index, id string) error {
	var span trace.Span
	if c.cfg != nil && c.cfg.EnableTrace {
		ctx, span = c.tracer.Start(ctx, "db.elasticsearch.delete")
		defer span.End()
	}
	if span != nil {
		span.SetAttributes(
			attribute.String("db.elasticsearch.index", index),
			attribute.String("db.elasticsearch.id", id),
		)
	}
	res, err := c.es.Delete(index, id)
	if err != nil {
		c.logger.ErrorContext(ctx, "searchx/es delete failed", logx.Err(err), logx.String("index", index), logx.String("id", id))
		return err
	}
	defer res.Body.Close()
	c.logger.InfoContext(ctx, "searchx/es delete done", logx.String("index", index), logx.String("id", id))
	return nil
}

func (c *ClientImpl) Search(ctx context.Context, index, query string) (*searchx.QueryResult, error) {
	var span trace.Span
	if c.cfg != nil && c.cfg.EnableTrace {
		ctx, span = c.tracer.Start(ctx, "db.elasticsearch.search")
		defer span.End()
	}
	if span != nil {
		span.SetAttributes(
			attribute.String("db.elasticsearch.index", index),
		)
	}
	var req map[string]interface{}
	if query != "" {
		// 用 multi_match + 通配字段，而不是 match _all：
		// _all 元字段在 ES 6 起废弃、7 起移除，对 7.x 查询恒返回 0 命中且不报错，
		// 是那种"看起来通了其实没检到"的静默失效。通配字段则与索引映射无关。
		req = map[string]interface{}{
			"query": map[string]interface{}{
				"multi_match": map[string]interface{}{
					"query":  query,
					"fields": []string{"*"},
				},
			},
		}
	} else {
		req = map[string]interface{}{
			"query": map[string]interface{}{"match_all": map[string]interface{}{}},
		}
	}
	body, _ := json.Marshal(req)
	res, err := c.es.Search(c.es.Search.WithIndex(index), c.es.Search.WithBody(bytes.NewReader(body)))
	if err != nil {
		c.logger.ErrorContext(ctx, "searchx/es search failed", logx.Err(err), logx.String("index", index))
		return nil, fmt.Errorf("searchx/es search: %w", err)
	}
	defer res.Body.Close()
	// 字段名必须带 json tag：ES 的响应键是 _source，而 Go 的无 tag 字段
	// 只会按 "Source" 去匹配，结果全部命中但 Source 恒为 nil —— 命中数对、内容空，
	// 比直接报错更难发现。
	var result struct {
		Hits struct {
			Total struct{ Value int }
			Hits  []struct {
				Source searchx.Document `json:"_source"`
			}
		}
	}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, err
	}
	hits := make([]searchx.Document, len(result.Hits.Hits))
	for i, h := range result.Hits.Hits {
		hits[i] = h.Source
	}
	c.logger.InfoContext(ctx, "searchx/es search done", logx.String("index", index), logx.Int("total", result.Hits.Total.Value))
	return &searchx.QueryResult{Total: result.Hits.Total.Value, Hits: hits}, nil
}

func (c *ClientImpl) Bulk(ctx context.Context, index string, ops []searchx.BulkOp) error {
	if len(ops) == 0 {
		return nil
	}
	var span trace.Span
	if c.cfg != nil && c.cfg.EnableTrace {
		ctx, span = c.tracer.Start(ctx, "db.elasticsearch.bulk")
		defer span.End()
	}
	if span != nil {
		span.SetAttributes(
			attribute.String("db.system", "elasticsearch"),
			attribute.String("db.elasticsearch.index", index),
			attribute.Int("db.elasticsearch.bulk.operation_count", len(ops)),
		)
	}
	var buf bytes.Buffer
	for _, op := range ops {
		switch op.Action {
		case "index", "create":
			meta := map[string]interface{}{op.Action: map[string]interface{}{"_index": index}}
			if op.ID != "" {
				meta[op.Action].(map[string]interface{})["_id"] = op.ID
			}
			b, _ := json.Marshal(meta)
			buf.Write(b)
			buf.WriteByte('\n')
			b, _ = json.Marshal(op.Doc)
			buf.Write(b)
			buf.WriteByte('\n')
		case "delete":
			meta := map[string]interface{}{"delete": map[string]interface{}{"_index": index}}
			if op.ID != "" {
				meta["delete"].(map[string]interface{})["_id"] = op.ID
			}
			b, _ := json.Marshal(meta)
			buf.Write(b)
			buf.WriteByte('\n')
		case "update":
			meta := map[string]interface{}{"update": map[string]interface{}{"_index": index}}
			if op.ID != "" {
				meta["update"].(map[string]interface{})["_id"] = op.ID
			}
			b, _ := json.Marshal(meta)
			buf.Write(b)
			buf.WriteByte('\n')
			b, _ = json.Marshal(map[string]interface{}{"doc": op.Doc})
			buf.Write(b)
			buf.WriteByte('\n')
		}
	}
	if buf.Len() == 0 {
		return nil
	}
	res, err := c.es.Bulk(bytes.NewReader(buf.Bytes()), c.es.Bulk.WithIndex(index))
	if err != nil {
		if span != nil {
			span.RecordError(err)
		}
		c.logger.ErrorContext(ctx, "searchx/es bulk failed", logx.Err(err), logx.String("index", index), logx.Int("count", len(ops)))
		return fmt.Errorf("searchx/es bulk: %w", err)
	}
	defer res.Body.Close()
	var result struct {
		Errors bool                     `json:"errors"`
		Items  []map[string]interface{} `json:"items"`
	}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		if span != nil {
			span.RecordError(err)
		}
		return fmt.Errorf("searchx/es bulk decode: %w", err)
	}
	if result.Errors {
		for i, item := range result.Items {
			if item == nil {
				continue
			}
			var action string
			switch {
			case item["index"] != nil:
				action = "index"
			case item["create"] != nil:
				action = "create"
			case item["delete"] != nil:
				action = "delete"
			case item["update"] != nil:
				action = "update"
			}
			if action == "" {
				continue
			}
			opItem := item[action].(map[string]interface{})
			if errInfo, ok := opItem["error"]; ok {
				c.logger.ErrorContext(ctx, "searchx/es bulk item error", logx.Any("item", i), logx.Any("error", errInfo))
			}
		}
		return fmt.Errorf("searchx/es bulk: %d item errors", len(result.Items))
	}
	c.logger.InfoContext(ctx, "searchx/es bulk done", logx.String("index", index), logx.Int("count", len(ops)))
	return nil
}

func (c *ClientImpl) HealthCheck() error {
	status, err := c.es.Cluster.Health()
	if err != nil {
		return err
	}
	_ = status
	return nil
}

func (c *ClientImpl) Close() error {
	if c.ht != nil {
		c.ht.CloseIdleConnections()
	}
	return nil
}
func (c *ClientImpl) Driver() string { return "elasticsearch" }

func init() {
	searchx.RegisterCreator("elasticsearch", func(cfg *searchx.Config) (searchx.SearchClient, error) {
		return New(cfg)
	})
}

func withRetry(fn func() error, maxRetries int) error {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			time.Sleep(time.Duration(100<<uint(i-1)) * time.Millisecond)
		}
		if lastErr = fn(); lastErr == nil {
			return nil
		}
	}
	return fmt.Errorf("failed to connect after %d retries: %w", maxRetries, lastErr)
}
