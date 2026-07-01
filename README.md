# Pod-OS Client Library

A standalone Go package for connecting to Pod-OS Actors and working with the Pod-OS and Actor Infrastructure Platform message protocol.

## Features

- **Connection Management**: TCP/UDP connection handling with retry logic and connection pooling
- **Message Protocol**: Full support for Pod-OS message encoding/decoding
- **Intent Types**: All Pod-OS intent types (StoreEvent, StoreData, GetEvent, LinkEvents, etc.)
- **Size-Safe Messages**: Built-in guards for malicious or accidental oversize messages (hard 2 GiB limit)
- **Message Validation**: Struct-level, payload-level, and wire-level validation with dual-audience (engineer + LLM) error output
- **Knowledge Base**: Embedded Pod-OS documentation and specifications
- **Automatic Reconnection**: Configurable reconnect with exponential backoff, connection state callbacks, and transparent send-side retry
- **Actor Health Checks**: `StatusRequest` / `Status` probe and reply helpers for non-Neural Memory socket Actors
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
            EventUniqueIdA: "a", EventUniqueIdB: "b",
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

## Reconnection & Connection State

When `ReconnectConfig` is enabled (the default), the client automatically handles connection recovery:

- **Send methods block during reconnect** — `SendMessage`, `SendMessageWithRaw`, and their internal variants wait for an in-progress reconnect to complete rather than failing immediately. If the reconnect succeeds the send proceeds transparently; if it fails or the caller's context expires, `ErrConnectionLost` is returned.
- **`SendControlMessage` does not wait** — Control messages are fire-and-forget and will fail immediately if the connection is down.
- **`Close()` prevents reconnect** — After `Close` returns the client will never attempt to reconnect, and any in-flight `waitForReconnect` callers are unblocked.

### Observing Connection State

Register a callback to be notified of every connection state transition:

```go
client.OnConnectionStateChange(func(state podos.ConnectionState, err error) {
    log.Printf("connection state: %s (err=%v)", state, err)
})
```

The `ConnectionState` values are:

| State | Error param | Meaning |
|---|---|---|
| `StateConnected` | `nil` | Reconnect succeeded (not emitted on initial connect) |
| `StateDisconnected` | cause | Connection was lost |
| `StateReconnecting` | trigger error | Reconnect attempt starting |
| `StateReconnectFailed` | last error | All reconnect attempts exhausted |

The callback is invoked synchronously in the reconnect path — keep it fast and non-blocking.

## App-Level Keepalive

The client sends periodic AIP `Keepalive` frames (message_type 18) on every connection it owns so long-lived gateway sockets are not reaped during idle periods. This is separate from TCP `SO_KEEPALIVE`.

| Field | Default | Description |
|---|---|---|
| `KeepaliveInterval` | `30s` | Interval between keepalive frames. Set to `0` or negative to disable. |

Keepalive uses envelope-only routing: `To=$system@<gateway>`, `From=<clientName>@<gateway>`. The loop starts after the GatewayId handshake, pauses while disconnected or reconnecting, and stops on `Close()`. When a connection pool is configured, idle pooled connections are pinged as well; checked-out connections are skipped.

## Actor Health Checks (Non-Neural Memory Actors)

Neural Memory Actors are typically probed with store/get intents. **Socket Actors** (custom gateway-connected services that do not expose NeuralMemory) use the lightweight AIP `StatusRequest` / `Status` pair instead:

| Intent | message_type | Role |
|---|---|---|
| `StatusRequest` | 110 | Inbound health probe (envelope + optional `_msg_id`) |
| `Status` | 3 | Health reply (`_status`, `_msg`, echoed `_msg_id`) |

Both intents are envelope-only — no NeuralMemory fields or payload are required.

### Responding to probes (Actor side)

An Actor that holds a long-lived gateway socket must enable concurrent mode so inbound probes can arrive while outbound work is in flight. Call `RespondToHealthChecks` immediately after `NewClient`:

```go
cfg := config.Config{
    Host:                 "gateway-lb.example.com",
    Port:                 "62312",
    GatewayActorName:     "zeroth.pod-os.com",
    ClientName:           "my-socket-actor",
    EnableConcurrentMode: true,
}

client, err := podos.NewClient(ctx, cfg)
if err != nil {
    log.Fatal(err)
}
defer client.Close()

podos.RespondToHealthChecks(client)
```

`RespondToHealthChecks` registers an unmatched-message handler on the background receiver. When a `StatusRequest` arrives that does not correlate to an outbound request, the client replies with a `Status` message:

- `To` / `From` are swapped from the probe
- `MessageId` echoes the probe's `_msg_id` so the prober can correlate
- `Response.Status` is `"OK"` and `Response.Message` is `"actor is healthy"`
- The reply is sent via `SendControlMessage` (fire-and-forget; no response expected)

For custom reply logic, call `BuildStatusHealthReply` directly and send the encoded frame yourself:

```go
client.SetUnmatchedMessageHandler(func(inbound *message.Message) {
    if inbound.Intent.Name != message.IntentType.StatusRequest.Name {
        return
    }
    reply := podos.BuildStatusHealthReply(client, inbound)
    // customize reply.Response before encoding, if needed
    socketMsg, _ := message.EncodeMessage(reply, uuid.New().String())
    _ = client.SendControlMessage(ctx, socketMsg)
})
```

Wire `UnmatchedMessageHandler` in `config.Config` before `NewClient` if you need the handler active from the first received frame.

### Sending probes (monitor side)

To check whether a socket Actor is healthy, send a `StatusRequest` with a unique `MessageId` and wait for the correlated `Status` response:

```go
probeID := uuid.New().String()
probe := &message.Message{
    Envelope: message.Envelope{
        To:         "my-socket-actor@zeroth.pod-os.com",
        From:       client.FromAddress(),
        Intent:     message.IntentType.StatusRequest,
        ClientName: client.ClientName(),
        MessageId:  probeID,
    },
}

resp, err := client.SendMessage(ctx, probe)
if err != nil {
    log.Fatal(err)
}
if resp.ProcessingStatus() != "OK" {
    log.Fatalf("unhealthy: %s", resp.Response.Message)
}
if resp.MessageId != probeID {
    log.Fatalf("correlation mismatch: got %q, want %q", resp.MessageId, probeID)
}
```

When `PODOS_VALIDATE=1` is set, `msg.Validate()` accepts envelope-only `StatusRequest` messages and warns if `_msg_id` is missing (responses would not be correlatable).

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

MIT License

Copyright (c) 2026 PointOfData

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.


