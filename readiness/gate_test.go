package readiness

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/PointOfData/pod-os-go-client/message"
)

func fastReadinessConfig() ActorAIPReadinessConfig {
	return ActorAIPReadinessConfig{
		Timeout:        200 * time.Millisecond,
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     2 * time.Millisecond,
	}
}

func TestWaitForActorAIPReady_SucceedsImmediately(t *testing.T) {
	var calls int
	send := func(_ context.Context, _ *message.Message, _ string) (*message.Message, error) {
		calls++
		return &message.Message{}, nil
	}

	err := WaitForActorAIPReady(context.Background(), send, "a@zeroth.pod-os.com", "c@zeroth.pod-os.com", "c", "socket", fastReadinessConfig())
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 probe, got %d", calls)
	}
}

func TestWaitForGatewayAIPReady_UsesProbeActor(t *testing.T) {
	var to string
	send := func(_ context.Context, msg *message.Message, _ string) (*message.Message, error) {
		to = msg.To
		return &message.Message{}, nil
	}
	probe := GatewayReadinessProbe{ProbeActor: "test@zeroth.pod-os.com", ProbeActorType: "neural_memory"}
	if err := WaitForGatewayAIPReady(context.Background(), send, probe, "c@zeroth.pod-os.com", "c", fastReadinessConfig()); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if to != "test@zeroth.pod-os.com" {
		t.Fatalf("probe To = %q, want test@zeroth.pod-os.com", to)
	}
}

func TestWaitForActorAIPReady_RetriesThenSucceeds(t *testing.T) {
	var calls int
	send := func(_ context.Context, _ *message.Message, _ string) (*message.Message, error) {
		calls++
		if calls < 3 {
			return nil, errors.New("connection to gateway was lost during request")
		}
		return &message.Message{}, nil
	}

	err := WaitForActorAIPReady(context.Background(), send, "a@zeroth.pod-os.com", "c@zeroth.pod-os.com", "c", "evolutionary-neural-memory", fastReadinessConfig())
	if err != nil {
		t.Fatalf("expected eventual success, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 probes, got %d", calls)
	}
}

func TestWaitForActorAIPReady_DeadlineExceeded(t *testing.T) {
	send := func(_ context.Context, _ *message.Message, _ string) (*message.Message, error) {
		return nil, errors.New("connection to gateway was lost during request")
	}

	err := WaitForActorAIPReady(context.Background(), send, "a@zeroth.pod-os.com", "c@zeroth.pod-os.com", "c", "socket", fastReadinessConfig())
	if err == nil {
		t.Fatal("expected deadline error, got nil")
	}
}

func TestWaitForActorAIPReady_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	send := func(_ context.Context, _ *message.Message, _ string) (*message.Message, error) {
		cancel()
		return nil, errors.New("connection to gateway was lost during request")
	}

	err := WaitForActorAIPReady(ctx, send, "a@zeroth.pod-os.com", "c@zeroth.pod-os.com", "c", "socket", ActorAIPReadinessConfig{
		Timeout:        5 * time.Second,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     20 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected context-cancel error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected wrapped context.Canceled, got %v", err)
	}
}
