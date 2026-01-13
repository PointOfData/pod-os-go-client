package knowledge

import _ "embed"

//go:embed docs/Pod-OS-Communication-Prompts.md
var CommunicationPrompts string

//go:embed docs/Pod-OS-Message-Handling-Prompts.md
var MessageHandlingPrompts string

//go:embed docs/Pod-OS-Neural-Memory-Event-Prompts.md
var NeuralMemoryEventPrompts string

//go:embed docs/Pod-OS-Neural-Memory-Retrieval-Prompts.md
var NeuralMemoryRetrievalPrompts string

//go:embed docs/Pod-OS-Plan.md
var Plan string

//go:embed docs/Pod-OS-Prompts.md
var Prompts string

//go:embed docs/research.md
var Research string
