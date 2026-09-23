// Package mq publishes raw pod metrics to RabbitMQ for the Python
// anomaly detector, and declares the exchanges and queues it relies on.
package mq

import (
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	// MetricsExchange is the topic exchange raw per-pod samples are
	// published to, routed by "<namespace>.<pod>".
	MetricsExchange = "metrics.raw"
	// MetricsQueue is the durable queue the Python detector consumes
	// from; declared here so samples published before the detector
	// first starts aren't dropped by an unbound exchange.
	MetricsQueue = "metrics.raw.detector"
	// MetricsDeadLetterExchange receives messages the detector nacks
	// without requeue (e.g. malformed payloads).
	MetricsDeadLetterExchange = "metrics.raw.dlx"
	// MetricsDeadLetterQueue holds dead-lettered metric messages for
	// inspection.
	MetricsDeadLetterQueue = "metrics.raw.dlq"
)

// Channel is the subset of *amqp.Channel the publisher depends on.
type Channel interface {
	// ExchangeDeclare idempotently declares an exchange.
	ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error
	// QueueDeclare idempotently declares a queue.
	QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error)
	// QueueBind binds a queue to an exchange with a routing key.
	QueueBind(name, key, exchange string, noWait bool, args amqp.Table) error
}

// Purpose: idempotently declares the metrics.raw topology — the topic
// exchange, the detector's queue, and its dead-letter exchange/queue.
// The Python detector declares the same topology with identical
// arguments, so either side can start first.
// Params:
//   - ch: an open AMQP channel.
//
// Returns: the first declaration error, or nil.
func DeclareTopology(ch Channel) error {
	if err := ch.ExchangeDeclare(MetricsDeadLetterExchange, amqp.ExchangeFanout, true, false, false, false, nil); err != nil {
		return err
	}
	if _, err := ch.QueueDeclare(MetricsDeadLetterQueue, true, false, false, false, nil); err != nil {
		return err
	}
	if err := ch.QueueBind(MetricsDeadLetterQueue, "", MetricsDeadLetterExchange, false, nil); err != nil {
		return err
	}

	if err := ch.ExchangeDeclare(MetricsExchange, amqp.ExchangeTopic, true, false, false, false, nil); err != nil {
		return err
	}
	queueArgs := amqp.Table{"x-dead-letter-exchange": MetricsDeadLetterExchange}
	if _, err := ch.QueueDeclare(MetricsQueue, true, false, false, false, queueArgs); err != nil {
		return err
	}
	// "#" matches every "<namespace>.<pod>" key; the detector wants all pods.
	return ch.QueueBind(MetricsQueue, "#", MetricsExchange, false, nil)
}
