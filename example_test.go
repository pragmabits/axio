package axio_test

import (
	"context"

	"github.com/pragmabits/axio"
)

// ExampleLogger_With_http documents the recommended way to attach HTTP
// metadata to a log line. It is also compiled by go test so the README/
// package doc cannot drift out of sync.
func ExampleLogger_With_http() {
	logger, _ := axio.New(axio.Config{
		ServiceName: "example",
		Environment: axio.Development,
		Level:       axio.LevelInfo,
	})
	defer logger.Close()

	logger.With(
		axio.Annotate("http", axio.HTTP{
			Method:     "POST",
			URL:        "/api/v1/orders",
			StatusCode: 201,
			LatencyMS:  45,
		}),
		axio.Annotate("user_id", "usr_123"),
	).Info(context.Background(), "order created")
}
