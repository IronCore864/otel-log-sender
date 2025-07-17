## Install signoz

https://signoz.io/docs/install/docker/

## A simple Python app with logs to otelcol to test signoz/otelcol

https://signoz.io/docs/userguide/python-logs-auto-instrumentation/

Run it:

```bash
OTEL_PYTHON_LOGGING_AUTO_INSTRUMENTATION_ENABLED=true \
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317 \
opentelemetry-instrument --traces_exporter otlp --metrics_exporter otlp --logs_exporter otlp python main.py
```

## A simple Go app with otelcol modules

```go
package main

import (
	"context"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/sdk/log"
)

func main() {
	ctx := context.Background()

	// Create the OTLP log exporter
	logExporter, err := otlploghttp.New(ctx)
	if err != nil {
		panic("failed to initialize exporter")
	}

	// Create the logger provider
	lp := log.NewLoggerProvider(
		log.WithProcessor(
			log.NewBatchProcessor(logExporter),
		),
	)

	// Ensure the logger is shutdown before exiting
	defer func() {
		if err := lp.Shutdown(ctx); err != nil {
			panic(err)
		}
	}()

	// Set the logger provider globally
	global.SetLoggerProvider(lp)

	// Instantiate a new slog logger
	logger := otelslog.NewLogger("my-service")

	// Use the logger
	logger.Debug("Something interesting happened")
}
```

Run:

```bash
OTEL_SERVICE_NAME="my-service" \
OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4318" \
go run main.go
```

## Go size increase with otelcol modules

1.8M -> 12M size increase with the following modules:

- "go.opentelemetry.io/contrib/bridges/otelslog"
- "go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
- "go.opentelemetry.io/otel/log/global"
- "go.opentelemetry.io/otel/sdk/log"

## Reading the code

Structs reference:

- opentelemetry-collector/pdata/internal/data/protogen/logs/v1/logs.pb.go
- opentelemetry-collector/pdata/internal/data/protogen/resource/v1/resource.pb.go
- opentelemetry-collector/pdata/internal/data/protogen/common/v1/common.pb.go

## Self-implemented version

Binary size: 5.4M.

## Build & run

```bash
go build -trimpath -ldflags='-s -w'
./otel-log-sender
```
