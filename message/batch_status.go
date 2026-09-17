package message

import (
	"fmt"
	"strings"
)

// BatchLinksFailed returns a non-nil error when a StoreBatchLinks response reports
// per-link failures. The envelope ProcessingStatus can be OK while the batch record
// carries Status=ERROR or _links_with_errors > 0.
func BatchLinksFailed(msg *Message) error {
	if msg == nil || msg.Response == nil {
		return nil
	}
	rec := msg.Response.StoreLinkBatchEventRecord
	if strings.EqualFold(strings.TrimSpace(rec.Status), "ERROR") || rec.LinksWithErrors > 0 {
		text := strings.TrimSpace(rec.Message)
		if text == "" {
			text = "link store failed"
		}
		return fmt.Errorf("%s", text)
	}
	return nil
}

// BatchEventsFailed returns a non-nil error when a StoreBatchEvents response reports
// batch record failures. The envelope ProcessingStatus can be OK while the batch record
// carries Status=ERROR.
func BatchEventsFailed(msg *Message) error {
	if msg == nil || msg.Response == nil {
		return nil
	}
	rec := msg.Response.StoreBatchEventRecord
	if strings.EqualFold(strings.TrimSpace(rec.Status), "ERROR") {
		text := strings.TrimSpace(rec.Message)
		if text == "" {
			text = "batch event store failed"
		}
		return fmt.Errorf("%s", text)
	}
	return nil
}
