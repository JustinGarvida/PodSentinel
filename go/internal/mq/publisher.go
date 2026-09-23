package mq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"podsentinel/internal/k8s"
)

const (
	// dialTimeout bounds a single TCP+AMQP handshake so a down broker
	// can't stall a poll cycle for long.
	dialTimeout = 5 * time.Second
	// minRedialBackoff is the wait after the first failed dial.
	minRedialBackoff = 1 * time.Second
	// maxRedialBackoff caps the doubling redial wait.
	maxRedialBackoff = 30 * time.Second
)

// ErrUnavailable is returned by Publish while the publisher is backing
// off after a failed dial; callers log it and move on.
var ErrUnavailable = errors.New("rabbitmq unavailable, backing off before redialing")

// session is one open connection + channel to the broker.
type session interface {
	// PublishWithContext publishes a single message.
	PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error
	// Close closes the channel and its connection.
	Close() error
}

// Publisher publishes one metrics.raw message per pod sample. It dials
// lazily and redials with exponential backoff, so the agent starts and
// keeps writing to Postgres even while RabbitMQ is down.
type Publisher struct {
	// dial opens a new session with the topology already declared.
	dial func() (session, error)
	// now returns the current time; overridable in tests.
	now    func() time.Time
	logger *slog.Logger

	mu         sync.Mutex
	sess       session
	backoff    time.Duration
	nextDialAt time.Time
}

// Purpose: constructs a Publisher for the broker at url. No connection
// is made until the first Publish call.
// Params:
//   - url: the AMQP URL (e.g. "amqp://guest:guest@localhost:5672/").
//   - logger: structured logger for connect/disconnect events.
//
// Returns: a ready-to-use *Publisher.
func NewPublisher(url string, logger *slog.Logger) *Publisher {
	return &Publisher{
		dial:   func() (session, error) { return dialSession(url) },
		now:    time.Now,
		logger: logger,
	}
}

// Purpose: publishes sample to metrics.raw with routing key
// "<namespace>.<pod>". A failed publish drops the session and retries
// once on a fresh connection, covering a broker restart between cycles.
// Params:
//   - ctx: bounds the publish call.
//   - sample: the pod sample to publish.
//
// Returns: nil on success; ErrUnavailable while backing off; otherwise
// the dial or publish error.
func (p *Publisher) Publish(ctx context.Context, sample k8s.PodSample) error {
	body, err := json.Marshal(NewMetricMessage(sample))
	if err != nil {
		return fmt.Errorf("encoding metric message: %w", err)
	}

	msg := amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Timestamp:    sample.Timestamp,
		Body:         body,
	}
	key := RoutingKey(sample.Namespace, sample.Name)

	p.mu.Lock()
	defer p.mu.Unlock()

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		sess, err := p.session()
		if err != nil {
			return err
		}
		if lastErr = sess.PublishWithContext(ctx, MetricsExchange, key, false, false, msg); lastErr == nil {
			return nil
		}
		p.logger.Warn("rabbitmq publish failed, dropping connection", "routing_key", key, "error", lastErr)
		p.dropSession()
	}
	return lastErr
}

// Purpose: closes the current session, if any. Safe to call more than
// once.
// Params: none.
// Returns: the session's close error, or nil.
func (p *Publisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.sess == nil {
		return nil
	}
	err := p.sess.Close()
	p.sess = nil
	return err
}

// Purpose: returns the open session, dialing a new one unless still
// inside the backoff window from a previous failed dial. Caller must
// hold p.mu.
// Params: none.
// Returns: the session, or ErrUnavailable / the dial error.
func (p *Publisher) session() (session, error) {
	if p.sess != nil {
		return p.sess, nil
	}
	if p.now().Before(p.nextDialAt) {
		return nil, ErrUnavailable
	}

	sess, err := p.dial()
	if err != nil {
		p.backoff = min(max(p.backoff*2, minRedialBackoff), maxRedialBackoff)
		p.nextDialAt = p.now().Add(p.backoff)
		p.logger.Error("connecting to rabbitmq failed", "retry_in", p.backoff.String(), "error", err)
		return nil, err
	}

	if p.backoff > 0 {
		p.logger.Info("reconnected to rabbitmq")
	}
	p.backoff = 0
	p.nextDialAt = time.Time{}
	p.sess = sess
	return sess, nil
}

// Purpose: closes and forgets the current session so the next publish
// redials. Caller must hold p.mu.
// Params: none.
// Returns: nothing; close errors are ignored since the session is
// already presumed broken.
func (p *Publisher) dropSession() {
	if p.sess != nil {
		_ = p.sess.Close()
		p.sess = nil
	}
}

// amqpSession is the real session backed by amqp091-go.
type amqpSession struct {
	conn *amqp.Connection
	*amqp.Channel
}

// Purpose: closes the channel, then the connection.
// Params: none.
// Returns: the connection's close error, or nil.
func (s *amqpSession) Close() error {
	_ = s.Channel.Close()
	return s.conn.Close()
}

// Purpose: dials the broker, opens a channel, and declares the
// metrics.raw topology on it.
// Params:
//   - url: the AMQP URL.
//
// Returns: the open session, or the first error encountered.
func dialSession(url string) (session, error) {
	conn, err := amqp.DialConfig(url, amqp.Config{Dial: amqp.DefaultDial(dialTimeout)})
	if err != nil {
		return nil, fmt.Errorf("dialing rabbitmq: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("opening channel: %w", err)
	}

	if err := DeclareTopology(ch); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("declaring topology: %w", err)
	}

	return &amqpSession{conn: conn, Channel: ch}, nil
}
