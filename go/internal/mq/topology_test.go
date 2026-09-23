package mq

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// testAMQPURL returns the RabbitMQ URL to test against — the
// docker-compose instance from infra/docker-compose.yml. Set
// TEST_RABBITMQ_URL to override.
func testAMQPURL() string {
	if url := os.Getenv("TEST_RABBITMQ_URL"); url != "" {
		return url
	}
	return "amqp://guest:guest@localhost:5672/"
}

// requireBroker skips the test if the docker-compose RabbitMQ instance
// (see infra/docker-compose.md) isn't reachable.
func requireBroker(t *testing.T) *amqp.Connection {
	t.Helper()
	url := testAMQPURL()
	conn, err := amqp.DialConfig(url, amqp.Config{Dial: amqp.DefaultDial(2 * time.Second)})
	if err != nil {
		t.Skipf("skipping: RabbitMQ not reachable at %s (is `docker compose -f infra/docker-compose.yml up -d` running?): %v", url, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestPublisher_DeliversToDetectorQueueOnRealBroker(t *testing.T) {
	conn := requireBroker(t)
	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("opening channel: %v", err)
	}

	if err := DeclareTopology(ch); err != nil {
		t.Fatalf("DeclareTopology() error = %v", err)
	}
	if _, err := ch.QueuePurge(MetricsQueue, false); err != nil {
		t.Fatalf("purging %s: %v", MetricsQueue, err)
	}

	p := NewPublisher(testAMQPURL(), testLogger())
	defer p.Close()

	sample := testSample()
	sample.Name = "topology-test-pod"
	if err := p.Publish(context.Background(), sample); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	var got amqp.Delivery
	var ok bool
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if got, ok, err = ch.Get(MetricsQueue, true); err != nil {
			t.Fatalf("Get(%s): %v", MetricsQueue, err)
		} else if ok {
			break
		}
	}
	if !ok {
		t.Fatalf("no message arrived on %s", MetricsQueue)
	}

	if got.RoutingKey != "default.topology-test-pod" {
		t.Errorf("routing key = %q, want %q", got.RoutingKey, "default.topology-test-pod")
	}
	var body MetricMessage
	if err := json.Unmarshal(got.Body, &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body.Pod != "topology-test-pod" || body.SchemaVersion != MetricMessageSchemaVersion {
		t.Errorf("body = %+v, want pod=topology-test-pod schema_version=%d", body, MetricMessageSchemaVersion)
	}
}

func TestDeclareTopology_RejectedMessagesLandInDeadLetterQueue(t *testing.T) {
	conn := requireBroker(t)
	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("opening channel: %v", err)
	}
	if err := DeclareTopology(ch); err != nil {
		t.Fatalf("DeclareTopology() error = %v", err)
	}
	for _, q := range []string{MetricsQueue, MetricsDeadLetterQueue} {
		if _, err := ch.QueuePurge(q, false); err != nil {
			t.Fatalf("purging %s: %v", q, err)
		}
	}

	err = ch.PublishWithContext(context.Background(), MetricsExchange, "default.malformed", false, false, amqp.Publishing{Body: []byte("not json")})
	if err != nil {
		t.Fatalf("publishing: %v", err)
	}

	var d amqp.Delivery
	var ok bool
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline) && !ok; time.Sleep(50 * time.Millisecond) {
		if d, ok, err = ch.Get(MetricsQueue, false); err != nil {
			t.Fatalf("Get(%s): %v", MetricsQueue, err)
		}
	}
	if !ok {
		t.Fatalf("no message arrived on %s", MetricsQueue)
	}
	// What the detector does with a payload it can't parse.
	if err := d.Nack(false, false); err != nil {
		t.Fatalf("Nack: %v", err)
	}

	ok = false
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline) && !ok; time.Sleep(50 * time.Millisecond) {
		if _, ok, err = ch.Get(MetricsDeadLetterQueue, true); err != nil {
			t.Fatalf("Get(%s): %v", MetricsDeadLetterQueue, err)
		}
	}
	if !ok {
		t.Fatalf("nacked message did not reach %s", MetricsDeadLetterQueue)
	}
}
