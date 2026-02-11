# Pod-OS Client Library

A standalone Go package for connecting to Pod-OS Actors and working with the Pod-OS and Actor Information Platform message protocol.

## Features

- **Connection Management**: TCP/UDP connection handling with retry logic and connection pooling
- **Message Protocol**: Full support for Pod-OS message encoding/decoding
- **Intent Types**: All Pod-OS intent types (StoreEvent, GetEvent, LinkEvents, etc.)
- **Size-Safe Messages**: Built-in guards for malicious or accidental oversize messages (hard 2 GiB limit)
- **Knowledge Base**: Embedded Pod-OS documentation and specifications
- **Zero Dashboard Dependencies**: No web framework or dashboard-specific code

## Installation

```bash
go get github.com/PointOfData/pod-os-go-client
```

## Quick Start

## Knowledge Base

Access embedded Pod-OS documentation:

```go
import "github.com/PointOfData/pod-os-go-client/knowledge"

// Get a specific document
doc, err := knowledge.GetDocument("neural-memory")
if err != nil {
    log.Fatal(err)
}
fmt.Println(doc)

// List all available documents
docs := knowledge.ListDocuments()
for _, name := range docs {
    fmt.Println(name)
}
```

## Logging and Telemetry

The client supports injectable logging and optional OpenTelemetry tracing for development, troubleshooting, and production monitoring.

### Message Size Limits and Error Handling

- The Pod-OS wire format allows large messages, but this client enforces a **maximum full message size of 2 GiB**, including length prefix, header, tags, and payload.
- When **encoding**, `message.EncodeMessage` returns an `EncodeError` with `ErrCodeEncodePayloadTooLarge` if either the payload or the encoded message would exceed this limit.
- When **decoding**, `message.DecodeMessage` returns a `DecodeError` with `ErrCodeDecodePayloadTooLarge` if the incoming message exceeds the limit, and `ErrCodeDecodeMessageTooShort` for inconsistent or truncated `to`, `from`, header, or payload regions (common bad-actor scenarios).
- Size-related encode/decode errors **fail only the current operation**; the underlying TCP connection is left open whenever possible so callers can decide how to recover.

### Logging

Configure logging via `config.Config`:

- **LogLevel**: 0=disabled, 1=Error, 2=Warn, 3=Info, 4=Debug. Use 1-2 for production, 3-4 for development.
- **Logger**: Inject your own `log.Logger` implementation. If nil, uses `NoOpLogger` (zero overhead).

When `Logger` is nil but `LogLevel` > 0, a default `slog`-based logger is created:

```go
cfg := config.Config{
    LogLevel: 3,  // Info level
    // Logger: nil - default slog logger to stderr
}

client, err := podos.NewClient(ctx, cfg)
```

For custom logging (e.g., zap, zerolog), implement the `log.Logger` interface and pass it:

```go
import "github.com/PointOfData/pod-os-go-client/log"

cfg := config.Config{
    Logger: myCustomLogger,
}
```

### OpenTelemetry Tracing

The client supports optional OpenTelemetry via the `connection.Tracer` interface. Implement the interface to bridge your OTLP tracer:

```go
// connection.Tracer interface:
//   Start(ctx context.Context, name string) (context.Context, Span)
// connection.Span interface:
//   End(), RecordError(err error), AddEvent(name string)

cfg := config.Config{
    TracerName:    "your-service-name",
    Tracer:        myTracer,  // Implements connection.Tracer
    EnableTracing: true,
}
```

The core module has no OpenTelemetry dependency. Applications that want tracing add `go.opentelemetry.io/otel` and pass a `Tracer` implementing the `connection.Tracer` interface. Spans are instrumented for Send, Receive, Reconnect, and Close operations.

## Package Structure

- `connection/` - Network connection management
- `message/` - Message protocol types and encoding/decoding
- `log/` - Logging interface and implementations
- `errors/` - Error definitions
- `config/` - Configuration structures
- `knowledge/` - Embedded Pod-OS documentation

## License

[Your License Here]

