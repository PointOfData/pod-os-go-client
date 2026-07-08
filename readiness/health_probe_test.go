package readiness

import (
	"testing"

	"github.com/PointOfData/pod-os-go-client/message"
)

func TestIsNeuralMemoryBackedForHealthProbe(t *testing.T) {
	nmTypes := []string{"neural_memory", "pod_db", "script", "mailbox", "evolutionary-neural-memory", "Neural_Memory"}
	for _, typ := range nmTypes {
		if !IsNeuralMemoryBackedForHealthProbe(typ) {
			t.Fatalf("%q should be neural-memory-backed", typ)
		}
	}
	nonNM := []string{"socket", "router", "shell", "", "gateway"}
	for _, typ := range nonNM {
		if IsNeuralMemoryBackedForHealthProbe(typ) {
			t.Fatalf("%q should not be neural-memory-backed", typ)
		}
	}
}

func TestBuildActorHealthProbeMessage_IntentByType(t *testing.T) {
	socketMsg := BuildActorHealthProbeMessage("mysocket@gateway.pod-os.com", "client@zeroth.pod-os.com", "client", "socket")
	if socketMsg.Intent.Name != message.IntentType.StatusRequest.Name {
		t.Fatalf("socket probe intent = %q, want StatusRequest", socketMsg.Intent.Name)
	}
	if socketMsg.MessageId == "" {
		t.Fatal("socket probe missing MessageId")
	}

	nmMsg := BuildActorHealthProbeMessage("account@zeroth.pod-os.com", "client@zeroth.pod-os.com", "client", "neural_memory")
	if nmMsg.Intent.Name != message.IntentType.GetEventsForTags.Name {
		t.Fatalf("NM probe intent = %q, want GetEventsForTags", nmMsg.Intent.Name)
	}
	if nmMsg.NeuralMemory == nil || nmMsg.NeuralMemory.GetEventsForTags == nil || !nmMsg.NeuralMemory.GetEventsForTags.CountOnly {
		t.Fatalf("NM probe missing CountOnly GetEventsForTags: %+v", nmMsg.NeuralMemory)
	}
}

func TestActorHealthProbeSucceeded(t *testing.T) {
	if !ActorHealthProbeSucceeded(nil, nil) {
		t.Fatal("expected success for nil err and nil resp")
	}
	if !ActorHealthProbeSucceeded(nil, &message.Message{}) {
		t.Fatal("expected success for non-ERROR response")
	}
	if ActorHealthProbeSucceeded(nil, &message.Message{Response: &message.ResponseFields{Status: "ERROR", Message: "fail"}}) {
		t.Fatal("expected failure for ERROR response")
	}
}
