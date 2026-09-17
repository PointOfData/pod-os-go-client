package message

import "testing"

func TestBatchLinksFailed(t *testing.T) {
	msg := &Message{
		Response: &ResponseFields{
			StoreLinkBatchEventRecord: StoreLinkBatchEventRecord{
				Status:          "ERROR",
				Message:         "OWNER EVENT NOT FOUND",
				LinksWithErrors: 2,
			},
		},
	}
	if err := BatchLinksFailed(msg); err == nil {
		t.Fatal("expected error for failed batch links")
	}
}

func TestBatchEventsFailed(t *testing.T) {
	msg := &Message{
		Response: &ResponseFields{
			StoreBatchEventRecord: StoreBatchEventRecord{
				Status:  "ERROR",
				Message: "missing owner",
			},
		},
	}
	if err := BatchEventsFailed(msg); err == nil {
		t.Fatal("expected error for failed batch events")
	}
}
