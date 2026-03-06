# Pod-OS Client Library

A standalone Go package for connecting to Pod-OS Actors and working with the Pod-OS and Actor Information Platform message protocol.

## Features

- **Connection Management**: TCP/UDP connection handling with retry logic and connection pooling
- **Message Protocol**: Full support for Pod-OS message encoding/decoding
- **Intent Types**: All Pod-OS intent types (StoreEvent, GetEvent, LinkEvents, etc.)
- **Size-Safe Messages**: Built-in guards for malicious or accidental oversize messages (hard 2 GiB limit)
- **Message Validation**: Struct-level, payload-level, and wire-level validation with dual-audience (engineer + LLM) error output
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

## Message Validation

The `message` package includes a comprehensive, opt-in validator that checks messages at three levels: struct fields, NeuralMemory payloads, and raw wire format. Validation is completely disabled by default so it never affects production throughput.

### Enabling validation

Set the `PODOS_VALIDATE` environment variable before starting your process:

```bash
# Enable (accepts "1", "true", or "yes", case-insensitive)
export PODOS_VALIDATE=1
```

The variable is read **once at package init**. Both `Validate()` and `ValidateRawMessage()` return `nil` immediately when the variable is not set — the hot path is a single bool check with zero allocations.

### Struct-level validation — `msg.Validate()`

Call `Validate()` on a fully constructed `*Message` before encoding it. The validator checks:

- **Envelope**: `To` and `From` in `name@gateway` format, `Intent` non-zero, `ClientName` for `GatewayId`
- **Per-intent required fields**: every NeuralMemory and Gateway/Actor intent has specific required fields checked with nil-guards
- **Payload contents**: batch intents (`StoreBatchEvents`, `StoreBatchTags`, `StoreBatchLinks`) validate per-record required fields

```go
msg := &message.Message{
    Envelope: message.Envelope{
        To:     "mem@zeroth.example.com",
        From:   "MyClient@zeroth.example.com",
        Intent: message.IntentType.LinkEvent,
    },
    NeuralMemory: &message.NeuralMemoryFields{
        Link: &message.LinkFields{
            EventA: "a", EventB: "b",
            Category: "related", StrengthA: 1.0, StrengthB: 1.0,
            Timestamp: "+1234567890.123456",
            OwnerID:   "owner-event-id",
            Location:  "TERRA|47.6|-122.5", LocationSeparator: "|",
        },
    },
}

if errs := msg.Validate(); len(errs) != 0 {
    log.Println(errs.Error())     // engineer-readable terminal output
    log.Println(errs.LLMJson())   // JSON array for LLM prompt injection
    return
}

socket, err := message.EncodeMessage(msg, uuid.New().String())
```

### Wire-level validation — `message.ValidateRawMessage(raw)`

Call `ValidateRawMessage` on any raw `[]byte` received from or about to be sent to an Actor. This validates the wire framing (length prefixes, `To`/`From` format, `messageType`) and per-intent header fields without needing to decode the full message first:

```go
if errs := message.ValidateRawMessage(raw); len(errs) != 0 {
    log.Println("malformed wire message:", errs.Error())
}
```

### Error output formats

Every `ValidationError` carries structured fields covering the Go struct path, wire protocol key, rule violated, human-readable description, concrete fix, and a minimal code example.

**Engineer format** — `errs.Error()` produces terminal-friendly lines:

```
[ERROR] LinkEvent / NeuralMemory.Link.Category (category): required
  What: NeuralMemory.Link.Category (category) is required for LinkEvent and is missing.
  Fix:  Set NeuralMemory.Link.Category to a non-empty relationship string.
  Code: msg.NeuralMemory.Link.Category = "related"

[WARN]  wire / Response.Status (_status): header_missing
  What: NeuralMemory response (messageType 1001) is missing _status header.
```

**LLM format** — `errs.LLMJson()` produces a JSON array suitable for injection into a prompt or tool-call response:

```json
[{
  "severity": "error",
  "intent": "LinkEvent",
  "struct_path": "NeuralMemory.Link.Category",
  "wire_field": "category",
  "rule": "required",
  "description": "NeuralMemory.Link.Category (category) is required for LinkEvent and is missing.",
  "fix": "Set NeuralMemory.Link.Category to a non-empty relationship string.",
  "example_code": "msg.NeuralMemory.Link.Category = \"related\"",
  "references": ["message/types.go:LinkFields.Category", "message/header.go:LinkEventsMessageHeader"]
}]
```

### AI-assisted remediation — `message.ExplainValidationErrors`

When a vLLM endpoint is available (OpenAI-compatible `/v1/chat/completions`), pass errors to `ExplainValidationErrors` for an AI-generated corrected code snippet:

```go
if len(errs) > 0 {
    explanation, err := message.ExplainValidationErrors(
        errs,
        "http://localhost:8000",              // vLLM base URL
        "meta-llama/Llama-3.1-8B-Instruct",  // model name
    )
    if err == nil {
        log.Println("AI suggestion:", explanation)
    }
}
```

The function is a no-op when `PODOS_VALIDATE` is not set or the error slice is empty.

### Typical dev/staging integration

```go
// Pre-send
if errs := msg.Validate(); len(errs) != 0 {
    log.Error(errs.Error())    // engineer output
    log.Debug(errs.LLMJson())  // structured output for tooling
    return errs
}
socket, err := message.EncodeMessage(msg, uuid.New().String())

// Post-receive
if errs := message.ValidateRawMessage(raw); len(errs) != 0 {
    log.Warn("wire validation:", errs.Error())
}
decoded, err := message.DecodeMessage(raw)
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
- `message/` - Message protocol types, encoding/decoding, and validation
- `log/` - Logging interface and implementations
- `errors/` - Error definitions
- `config/` - Configuration structures
- `knowledge/` - Embedded Pod-OS documentation

## License

[Your License Here]

