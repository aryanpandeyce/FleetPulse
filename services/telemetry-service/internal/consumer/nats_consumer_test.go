package consumer

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/fleetpulse/fleetpulse/pkg/logging"
	"github.com/fleetpulse/fleetpulse/services/telemetry-service/internal/domain"
	"github.com/fleetpulse/fleetpulse/services/telemetry-service/internal/engine"
	"github.com/fleetpulse/fleetpulse/services/telemetry-service/internal/repo"
	"github.com/nats-io/nats.go"
)

type mockBus struct {
	subscribed bool
	handler    nats.MsgHandler
	published  map[string][][]byte
}

func (m *mockBus) QueueSubscribe(subj, queue string, cb nats.MsgHandler) (*nats.Subscription, error) {
	m.subscribed = true
	m.handler = cb
	return &nats.Subscription{}, nil
}

func (m *mockBus) Publish(subj string, data []byte) error {
	if m.published == nil {
		m.published = make(map[string][][]byte)
	}
	m.published[subj] = append(m.published[subj], data)
	return nil
}

func TestConsumerHandleMessage(t *testing.T) {
	ring := repo.NewRingBuffer(10)
	evaluator := engine.NewRuleEvaluator()
	anomalies := engine.NewAnomalyEngine(15, 3.0, 0.15)

	defaultCons := NewConsumer(Config{
		Ring:      ring,
		Evaluator: evaluator,
		Anomalies: anomalies,
	})
	if defaultCons.subject != "telemetry.>" || defaultCons.queueGroup != "telemetry-workers" {
		t.Fatalf("unexpected defaults: %+v", defaultCons)
	}

	logger := logging.New("consumer-test", slog.LevelInfo)
	bus := &mockBus{}
	cons := NewConsumer(Config{
		Conn:      bus,
		Ring:      ring,
		Evaluator: evaluator,
		Anomalies: anomalies,
		Logger:    logger,
	})

	if err := cons.Start(context.Background()); err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}
	defer cons.Stop()

	if !bus.subscribed {
		t.Fatalf("expected bus to be subscribed")
	}

	// 1. Valid telemetry message
	sample := domain.TelemetrySample{
		DeviceID:  "dev-100",
		Serial:    "UCS-100",
		OrgID:     "org-test",
		CPUTemp:   52.0,
		InletTemp: 22.0,
		FanRPM:    [4]float64{4500, 4500, 4500, 4500},
	}
	data, _ := json.Marshal(sample)
	cons.HandleMessage(data)

	// Verify sample was saved in ring buffer
	recent := ring.GetRecent("dev-100", 1)
	if len(recent) != 1 || recent[0].CPUTemp != 52.0 {
		t.Fatalf("expected sample in ring buffer, got %+v", recent)
	}

	// 2. Alarm triggering message (fan failure)
	sampleAlarm := domain.TelemetrySample{
		DeviceID:  "dev-alarm",
		Serial:    "UCS-ALM",
		OrgID:     "org-test",
		CPUTemp:   50.0,
		InletTemp: 22.0,
		FanRPM:    [4]float64{0, 4500, 4500, 4500},
	}
	dataAlarm, _ := json.Marshal(sampleAlarm)
	cons.HandleMessage(dataAlarm)

	// 3. Anomaly triggering message (spike after warm-up)
	for i := 0; i < 20; i++ {
		cons.HandleMessage(data)
	}
	sampleSpike := domain.TelemetrySample{
		DeviceID:  "dev-100",
		Serial:    "UCS-100",
		OrgID:     "org-test",
		CPUTemp:   95.0,
		InletTemp: 22.0,
		FanRPM:    [4]float64{4500, 4500, 4500, 4500},
	}
	dataSpike, _ := json.Marshal(sampleSpike)
	cons.HandleMessage(dataSpike)

	// 4. Poison message (invalid JSON) - routes to DLQ path and logs warning
	cons.HandleMessage([]byte("{not-a-valid-json-message-at-all"))

	// 5. Test Stop with active sub
	cons.sub = &nats.Subscription{}
	cons.Stop()
}

