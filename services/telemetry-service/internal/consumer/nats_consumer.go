// Package consumer implements the NATS message consumer subscribing to telemetry subjects.
package consumer

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/fleetpulse/fleetpulse/services/telemetry-service/internal/domain"
	"github.com/fleetpulse/fleetpulse/services/telemetry-service/internal/engine"
	"github.com/fleetpulse/fleetpulse/services/telemetry-service/internal/repo"
	"github.com/nats-io/nats.go"
)

// MessageBus defines the messaging operations needed by the consumer.
type MessageBus interface {
	QueueSubscribe(subj, queue string, cb nats.MsgHandler) (*nats.Subscription, error)
	Publish(subj string, data []byte) error
}

// Consumer processes telemetry streams from NATS message bus.
type Consumer struct {
	bus        MessageBus
	sub        *nats.Subscription
	ring       *repo.RingBuffer
	evaluator  *engine.RuleEvaluator
	anomalies  *engine.AnomalyEngine
	logger     *slog.Logger
	queueGroup string
	subject    string
	dlqSubject string
	mu         sync.Mutex
	stopChan   chan struct{}
}

// Config configures the NATS telemetry consumer.
type Config struct {
	Conn       MessageBus
	Ring       *repo.RingBuffer
	Evaluator  *engine.RuleEvaluator
	Anomalies  *engine.AnomalyEngine
	Logger     *slog.Logger
	Subject    string
	QueueGroup string
	DLQSubject string
}

// NewConsumer constructs a telemetry consumer.
func NewConsumer(cfg Config) *Consumer {
	subject := cfg.Subject
	if subject == "" {
		subject = "telemetry.>"
	}
	queueGroup := cfg.QueueGroup
	if queueGroup == "" {
		queueGroup = "telemetry-workers"
	}
	dlqSubject := cfg.DLQSubject
	if dlqSubject == "" {
		dlqSubject = "telemetry.dlq"
	}

	return &Consumer{
		bus:        cfg.Conn,
		ring:       cfg.Ring,
		evaluator:  cfg.Evaluator,
		anomalies:  cfg.Anomalies,
		logger:     cfg.Logger,
		subject:    subject,
		queueGroup: queueGroup,
		dlqSubject: dlqSubject,
		stopChan:   make(chan struct{}),
	}
}

// Start begins consuming telemetry messages.
func (c *Consumer) Start(ctx context.Context) error {
	if c.bus == nil {
		if c.logger != nil {
			c.logger.Info("NATS connection nil, consumer in passive mode")
		}
		return nil
	}

	sub, err := c.bus.QueueSubscribe(c.subject, c.queueGroup, func(msg *nats.Msg) {
		c.HandleMessage(msg.Data)
	})
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.sub = sub
	c.mu.Unlock()

	return nil
}

// HandleMessage parses a raw telemetry message, pushes to buffer, and triggers engines.
func (c *Consumer) HandleMessage(data []byte) {
	var sample domain.TelemetrySample
	if err := json.Unmarshal(data, &sample); err != nil {
		if c.logger != nil {
			c.logger.Warn("poison telemetry message, routing to DLQ", "error", err)
		}
		if c.bus != nil {
			_ = c.bus.Publish(c.dlqSubject, data)
		}
		return
	}

	// 1. Ingest into fast ring buffer
	c.ring.Push(sample)

	// 2. Evaluate threshold alarm rules
	newAlarms := c.evaluator.Evaluate(sample)
	if len(newAlarms) > 0 && c.logger != nil {
		for _, a := range newAlarms {
			c.logger.Info("alarm raised",
				"alarm_id", a.ID,
				"serial", a.Serial,
				"rule", a.RuleName,
				"severity", a.Severity,
			)
		}
	}

	// 3. Process statistical anomaly detection
	detectedAnomalies := c.anomalies.Process(sample)
	if len(detectedAnomalies) > 0 && c.logger != nil {
		for _, anom := range detectedAnomalies {
			c.logger.Warn("anomaly detected",
				"serial", anom.Serial,
				"metric", anom.MetricName,
				"score", anom.AnomalyScore,
			)
		}
	}
}

// Stop unsubscribes and terminates consumption.
func (c *Consumer) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.sub != nil {
		_ = c.sub.Unsubscribe()
	}
}
