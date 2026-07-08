package readiness

import (
	"fmt"
	"strings"

	"github.com/PointOfData/pod-os-go-client/message"
	"github.com/google/uuid"
)

// IsNeuralMemoryBackedForHealthProbe reports whether an actor type runs pod_aip_db and
// can answer Neural-Memory intents such as GetEventsForTags.
func IsNeuralMemoryBackedForHealthProbe(actorType string) bool {
	switch strings.ToLower(strings.TrimSpace(actorType)) {
	case "pod_db", "evolutionary-neural-memory", "neural_memory", "neural-memory":
		return true
	default:
		return false
	}
}

// BuildActorHealthProbeMessage constructs the AIP health probe for one actor based on type.
// NM-backed actors use GetEventsForTags (CountOnly); socket/shell and other types use StatusRequest.
func BuildActorHealthProbeMessage(actorAddress, fromAddress, clientName, actorType string) message.Message {
	messageID := uuid.New().String()

	if IsNeuralMemoryBackedForHealthProbe(actorType) {
		healthCheckTag := fmt.Sprintf("_podos_health_check_%s", uuid.New().String())
		searchClause := fmt.Sprintf("health_check=%s", healthCheckTag)
		return message.Message{
			Envelope: message.Envelope{
				To:         actorAddress,
				From:       fromAddress,
				Intent:     message.IntentType.GetEventsForTags,
				ClientName: clientName,
				MessageId:  messageID,
			},
			Payload: &message.PayloadFields{
				Data: searchClause,
			},
			NeuralMemory: &message.NeuralMemoryFields{
				GetEventsForTags: &message.GetEventsForTagsOptions{
					CountOnly: true,
				},
			},
		}
	}

	return message.Message{
		Envelope: message.Envelope{
			To:         actorAddress,
			From:       fromAddress,
			Intent:     message.IntentType.StatusRequest,
			ClientName: clientName,
			MessageId:  messageID,
		},
	}
}

// ActorHealthProbeSucceeded reports whether a health probe transport and AIP status indicate success.
func ActorHealthProbeSucceeded(err error, resp *message.Message) bool {
	return err == nil && (resp == nil || resp.ProcessingStatus() != "ERROR")
}
