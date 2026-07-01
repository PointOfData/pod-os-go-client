package podos

import (
	"context"
	"time"

	"github.com/PointOfData/pod-os-go-client/log"
	"github.com/PointOfData/pod-os-go-client/message"
	"github.com/google/uuid"
)

const defaultHealthReplyTimeout = 5 * time.Second

// RespondToHealthChecks registers an unmatched-message handler on c that replies to
// inbound StatusRequest probes with a Status message echoing the request MessageId.
// Requires EnableConcurrentMode so the background receiver is active.
func RespondToHealthChecks(c *Client) {
	if c == nil {
		return
	}
	c.SetUnmatchedMessageHandler(func(inbound *message.Message) {
		if inbound == nil || inbound.Intent.Name != message.IntentType.StatusRequest.Name {
			return
		}
		reply := BuildStatusHealthReply(c, inbound)
		ctx, cancel := context.WithTimeout(context.Background(), defaultHealthReplyTimeout)
		defer cancel()
		socketMsg, err := message.EncodeMessage(reply, uuid.New().String())
		if err != nil {
			if c.logger.Enabled(log.LevelDebug) {
				c.logger.Debug("health reply encode failed", "error", err)
			}
			return
		}
		if err := c.SendControlMessage(ctx, socketMsg); err != nil {
			if c.logger.Enabled(log.LevelDebug) {
				c.logger.Debug("health reply send failed", "error", err)
			}
		}
	})
}

// BuildStatusHealthReply constructs a Status response for an inbound StatusRequest probe.
func BuildStatusHealthReply(c *Client, inbound *message.Message) *message.Message {
	requestID := ""
	to := ""
	if inbound != nil {
		requestID = inbound.MessageId
		to = inbound.From
	}
	reply := &message.Message{
		Envelope: message.Envelope{
			To:         to,
			From:       c.FromAddress(),
			Intent:     message.IntentType.Status,
			ClientName: c.ClientName(),
			MessageId:  requestID,
		},
		Response: &message.ResponseFields{
			Status:  "OK",
			Message: "actor is healthy",
		},
	}
	return reply
}
