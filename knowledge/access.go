package knowledge

import "fmt"

// GetDocument returns a specific knowledge base document
func GetDocument(name string) (string, error) {
	switch name {
	case "communication":
		return CommunicationPrompts, nil
	case "message-handling":
		return MessageHandlingPrompts, nil
	case "neural-memory":
		return NeuralMemoryEventPrompts, nil
	case "neural-memory-retrieval":
		return NeuralMemoryRetrievalPrompts, nil
	default:
		return "", fmt.Errorf("unknown document: %s", name)
	}
}

// ListDocuments returns all available document names
func ListDocuments() []string {
	return []string{
		"communication",
		"message-handling",
		"neural-memory",
		"neural-memory-retrieval",
		"plan",
		"prompts",
		"research",
	}
}
