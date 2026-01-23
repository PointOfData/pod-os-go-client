# Pod-OS Client Library

A standalone Go package for connecting to Pod-OS Actors and working with the Pod-OS and Actor Information Platform message protocol.

## Features

- **Connection Management**: TCP/UDP connection handling with retry logic and connection pooling
- **Message Protocol**: Full support for Pod-OS message encoding/decoding
- **Intent Types**: All Pod-OS intent types (StoreEvent, GetEvent, LinkEvents, etc.)
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

## Package Structure

- `connection/` - Network connection management
- `message/` - Message protocol types and encoding/decoding
- `errors/` - Error definitions
- `config/` - Configuration structures
- `knowledge/` - Embedded Pod-OS documentation

## License

[Your License Here]

