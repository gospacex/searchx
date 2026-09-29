// searchx/elasticsearch - 驱动注册
package elasticsearch

import (
	"github.com/gospacex/searchx"
)

func init() {
	searchx.RegisterCreator("elasticsearch", func(cfg *searchx.Config) (searchx.SearchClient, error) {
		return New(cfg)
	})
}
