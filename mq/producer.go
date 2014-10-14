package mq

import (
	log "github.com/cihub/seelog"
	"github.com/streadway/amqp"
)

type Producer struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	exchangeName string
	routingKey string
}

func NewProducer(amqpURI, exchangeName, exchangeType, routingKey string, reliable bool) (*Producer, error) {
	p := &Producer{
		conn: nil,
		channel: nil,
		exchangeName: exchangeName,
		routingKey: routingKey,
	}

	var err error
	p.conn, err = amqp.Dial(amqpURI)
	if err != nil {
		return nil, log.Errorf("failed to dial mq: %s", err)
	}

	p.channel, err = p.conn.Channel()
	if err != nil {
		return nil, log.Errorf("failed to get channel: %s", err)
	}

	if err = p.channel.ExchangeDeclare(
		exchangeName, // name
		exchangeType, // type
		true,         // durable
		false,        // auto-deleted
		false,        // internal
		false,        // noWait
		nil,          // arguments
	); err != nil {
		return nil, log.Errorf("failed to declare exchange: %s", err)
	}

	/*if reliable {
		if err = p.channel.Confirm(false); err != nil {
			return nil, log.Errorf("failed to put channel into confirm mode: %s", err)
		}
	}*/
	return p, nil
}

func (p *Producer) Publish(data []byte) error {
	//ack, nack := p.channel.NotifyConfirm(make(chan uint64, 1), make(chan uint64, 1))
	//defer confirmOne(ack, nack)

	if err := p.channel.Publish(
		p.exchangeName,   // publish to an exchange
		p.routingKey, // routing to 0 or more queues
		false,      // mandatory
		false,      // immediate
		amqp.Publishing{
			Headers:         amqp.Table{},
			ContentType:     "text/plain",
			ContentEncoding: "",
			Body:            data,
			DeliveryMode:    amqp.Transient, // 1=non-persistent, 2=persistent
			Priority:        0,              // 0-9
			// a bunch of application/implementation-specific fields
		},
	); err != nil {
		return log.Errorf("Exchange Publish: %s", err)
	}
	return nil
}

func (p *Producer) Shutdown() {
	p.channel.Close()
	p.conn.Close()
}

