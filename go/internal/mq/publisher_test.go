package mq

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"podsentinel/internal/k8s"
)

type published struct {
	exchange, key string
	msg           amqp.Publishing
}

type fakeSession struct {
	publishes []published
	failNext  int // number of upcoming publishes to fail
	closed    bool
}

func (f *fakeSession) PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error {
	if f.failNext > 0 {
		f.failNext--
		return errors.New("simulated publish failure")
	}
	f.publishes = append(f.publishes, published{exchange, key, msg})
	return nil
}

func (f *fakeSession) Close() error {
	f.closed = true
	return nil
}

// fakeDialer hands out sessions in order; a nil entry simulates a
// failed dial.
type fakeDialer struct {
	sessions []*fakeSession
	calls    int
}

func (d *fakeDialer) dial() (session, error) {
	i := d.calls
	d.calls++
	if i >= len(d.sessions) || d.sessions[i] == nil {
		return nil, errors.New("simulated dial failure")
	}
	return d.sessions[i], nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestPublisher(d *fakeDialer, clock *time.Time) *Publisher {
	return &Publisher{
		dial:   d.dial,
		now:    func() time.Time { return *clock },
		logger: testLogger(),
	}
}

func testSample() k8s.PodSample {
	return k8s.PodSample{
		Namespace: "default", Name: "web-7d9f8c6b5d-x8k2p", UID: "uid-1",
		OwnerKind: "Deployment", OwnerName: "web", Status: "Running",
		RestartCount: 2, CPU: 0.25, Memory: 1.5e8,
		Timestamp: time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC),
	}
}

func TestPublish_SendsOnePerPodMessageToMetricsExchange(t *testing.T) {
	sess := &fakeSession{}
	clock := time.Now()
	p := newTestPublisher(&fakeDialer{sessions: []*fakeSession{sess}}, &clock)

	if err := p.Publish(context.Background(), testSample()); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	if len(sess.publishes) != 1 {
		t.Fatalf("len(publishes) = %d, want 1", len(sess.publishes))
	}
	got := sess.publishes[0]
	if got.exchange != MetricsExchange || got.key != "default.web-7d9f8c6b5d-x8k2p" {
		t.Errorf("exchange/key = %q/%q, want %q/%q", got.exchange, got.key, MetricsExchange, "default.web-7d9f8c6b5d-x8k2p")
	}
	if got.msg.ContentType != "application/json" || got.msg.DeliveryMode != amqp.Persistent {
		t.Errorf("msg properties = %q/%d, want application/json/persistent", got.msg.ContentType, got.msg.DeliveryMode)
	}

	var body MetricMessage
	if err := json.Unmarshal(got.msg.Body, &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	want := NewMetricMessage(testSample())
	if body != want {
		t.Errorf("body = %+v, want %+v", body, want)
	}
}

func TestPublish_ReusesSessionAcrossCalls(t *testing.T) {
	d := &fakeDialer{sessions: []*fakeSession{{}}}
	clock := time.Now()
	p := newTestPublisher(d, &clock)

	for i := 0; i < 3; i++ {
		if err := p.Publish(context.Background(), testSample()); err != nil {
			t.Fatalf("Publish() #%d error = %v", i, err)
		}
	}

	if d.calls != 1 {
		t.Errorf("dial calls = %d, want 1", d.calls)
	}
}

func TestPublish_RedialsOnceAfterPublishFailure(t *testing.T) {
	stale := &fakeSession{failNext: 1}
	fresh := &fakeSession{}
	clock := time.Now()
	p := newTestPublisher(&fakeDialer{sessions: []*fakeSession{stale, fresh}}, &clock)

	if err := p.Publish(context.Background(), testSample()); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	if !stale.closed {
		t.Error("stale session was not closed")
	}
	if len(fresh.publishes) != 1 {
		t.Errorf("len(fresh.publishes) = %d, want 1", len(fresh.publishes))
	}
}

func TestPublish_BacksOffAfterFailedDialWithoutRedialing(t *testing.T) {
	d := &fakeDialer{sessions: []*fakeSession{nil, {}}}
	clock := time.Now()
	p := newTestPublisher(d, &clock)

	if err := p.Publish(context.Background(), testSample()); err == nil {
		t.Fatal("first Publish() error = nil, want dial error")
	}
	if err := p.Publish(context.Background(), testSample()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Publish() inside backoff error = %v, want ErrUnavailable", err)
	}
	if d.calls != 1 {
		t.Fatalf("dial calls inside backoff = %d, want 1", d.calls)
	}

	clock = clock.Add(minRedialBackoff)
	if err := p.Publish(context.Background(), testSample()); err != nil {
		t.Fatalf("Publish() after backoff error = %v", err)
	}
	if d.calls != 2 {
		t.Errorf("dial calls after backoff = %d, want 2", d.calls)
	}
}

func TestPublish_BackoffDoublesAndCaps(t *testing.T) {
	d := &fakeDialer{} // every dial fails
	clock := time.Now()
	p := newTestPublisher(d, &clock)

	var got []time.Duration
	for i := 0; i < 7; i++ {
		_ = p.Publish(context.Background(), testSample())
		got = append(got, p.backoff)
		clock = clock.Add(p.backoff)
	}

	want := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second, 30 * time.Second}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("backoffs = %v, want %v", got, want)
		}
	}
}

func TestClose_ClosesSessionAndIsIdempotent(t *testing.T) {
	sess := &fakeSession{}
	clock := time.Now()
	p := newTestPublisher(&fakeDialer{sessions: []*fakeSession{sess}}, &clock)
	_ = p.Publish(context.Background(), testSample())

	if err := p.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if !sess.closed {
		t.Error("session was not closed")
	}
}
