package readiness

import (
	"context"
	"fmt"
	"time"

	"github.com/PointOfData/pod-os-go-client/message"
)

// SendFunc performs one AIP probe send. Callers wire this to their client stack; the
// readiness loop owns retry/backoff and uses a short per-attempt timeout on each send.
type SendFunc func(ctx context.Context, msg *message.Message, label string) (*message.Message, error)

// ActorAIPReadinessConfig tunes the readiness polling loop. Zero fields fall back to
// defaults tuned to absorb gateway/peer-process startup latency (~60s budget).
type ActorAIPReadinessConfig struct {
	Timeout        time.Duration
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	// RequiredConsecutive is the number of back-to-back successful probes required
	// before the actor/gateway is considered ready. Values <=1 return on first
	// success (default). Values >1 turn this into a stabilization gate that rides out
	// a freshly-restarted gateway flapping (accept → reset) during a peer-route rollout.
	RequiredConsecutive int
	// SuccessInterval is the pause between consecutive probes once a success streak has
	// started. Ignored when RequiredConsecutive <=1.
	SuccessInterval time.Duration
}

func (c ActorAIPReadinessConfig) normalized() ActorAIPReadinessConfig {
	if c.Timeout <= 0 {
		c.Timeout = 60 * time.Second
	}
	if c.InitialBackoff <= 0 {
		c.InitialBackoff = 2 * time.Second
	}
	if c.MaxBackoff <= 0 {
		c.MaxBackoff = 8 * time.Second
	}
	if c.RequiredConsecutive <= 0 {
		c.RequiredConsecutive = 1
	}
	if c.SuccessInterval <= 0 {
		c.SuccessInterval = 2 * time.Second
	}
	return c
}

// GatewayReadinessProbe names a known-stable anchor actor used to confirm a gateway
// route is serving AIP again. Use a platform singleton (e.g. test@zeroth) — never a
// freshly-peered customer gateway that has no actors yet.
type GatewayReadinessProbe struct {
	ProbeActor     string
	ProbeActorType string
}

// WaitForActorAIPReady polls until the named actor answers an AIP health probe, or
// the budget elapses. actorType selects the probe intent via BuildActorHealthProbeMessage.
func WaitForActorAIPReady(ctx context.Context, send SendFunc, actorAddress, fromAddress, clientName, actorType string, rc ActorAIPReadinessConfig) error {
	return waitForAIPReady(ctx, send, actorAddress, fromAddress, clientName, actorType, rc)
}

// WaitForGatewayAIPReady polls until the stable anchor actor in probe answers an AIP
// health probe, confirming the gateway route is routable again.
func WaitForGatewayAIPReady(ctx context.Context, send SendFunc, probe GatewayReadinessProbe, fromAddress, clientName string, rc ActorAIPReadinessConfig) error {
	if probe.ProbeActor == "" {
		return fmt.Errorf("gateway readiness probe: ProbeActor is required")
	}
	return waitForAIPReady(ctx, send, probe.ProbeActor, fromAddress, clientName, probe.ProbeActorType, rc)
}

func waitForAIPReady(ctx context.Context, send SendFunc, actorAddress, fromAddress, clientName, actorType string, rc ActorAIPReadinessConfig) error {
	if send == nil {
		return fmt.Errorf("gateway readiness: nil send function")
	}
	rc = rc.normalized()
	backoff := rc.InitialBackoff
	deadline := time.Now().Add(rc.Timeout)
	var lastErr error
	consecutive := 0

	for attempt := 1; time.Now().Before(deadline); attempt++ {
		probeMsg := BuildActorHealthProbeMessage(actorAddress, fromAddress, clientName, actorType)
		aip, err := send(ctx, &probeMsg, "aip_ready_"+actorAddress)
		if ActorHealthProbeSucceeded(err, aip) {
			consecutive++
			if consecutive >= rc.RequiredConsecutive {
				return nil
			}
			backoff = rc.InitialBackoff
			select {
			case <-ctx.Done():
				return fmt.Errorf("actor %s AIP readiness aborted after %d attempt(s): %w", actorAddress, attempt, ctx.Err())
			case <-time.After(rc.SuccessInterval):
			}
			continue
		}
		consecutive = 0
		switch {
		case err != nil:
			lastErr = err
		case aip != nil:
			lastErr = fmt.Errorf("actor returned error: %s", aip.ProcessingMessage())
		default:
			lastErr = fmt.Errorf("probe returned no response")
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("actor %s AIP readiness aborted after %d attempt(s): %w", actorAddress, attempt, ctx.Err())
		case <-time.After(backoff):
		}
		if backoff < rc.MaxBackoff {
			backoff *= 2
			if backoff > rc.MaxBackoff {
				backoff = rc.MaxBackoff
			}
		}
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("deadline exceeded")
	}
	return fmt.Errorf("actor %s not reachable over AIP within %s: %w", actorAddress, rc.Timeout, lastErr)
}
