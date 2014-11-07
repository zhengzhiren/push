package main

import (
	log "github.com/cihub/seelog"
	"github.com/streadway/amqp"
)

var (
	amqpURI      string = "amqp://guest:guest@10.154.156.121:5672/"
	exchange     string = "gibbon_rpc_exchange"
	exchangeType string = "fanout"
	routingKey   string = ""
)

func control(service string, cmd string) (string, error) {
	conn, err := amqp.Dial(amqpURI)
	if err != nil {
		return log.Errorf("Dial: %s", err)
	}
	defer conn.Close()

	log.Printf("got Connection, getting Channel")
	channel, err := conn.Channel()
	if err != nil {
		return log.Errorf("Channel: %s", err)
	}

	log.Printf("got Channel, declaring %q Exchange (%q)", exchangeType, exchange)

	if err := channel.ExchangeDeclare(
		exchange,     // name
		exchangeType, // type
		true,         // durable
		false,        // auto-deleted
		false,        // internal
		false,        // noWait
		nil,          // arguments
	); err != nil {
		return log.Errorf("Exchange Declare: %s", err)
	}

	callbackQueue, err := channel.QueueDeclare(
		"",    // name
		false, // durable
		true,  // autoDelete
	)

	log.Printf("declared Exchange, publishing %dB cmd (%q)", len(cmd), cmd)
	if err = channel.Publish(
		exchange,   // publish to an exchange
		routingKey, // routing to 0 or more queues
		false,      // mandatory
		false,      // immediate
		amqp.Publishing{
			Headers:         amqp.Table{},
			ContentType:     "text/plain",
			ContentEncoding: "",
			DeliveryMode:    amqp.Transient, // 1=non-persistent, 2=persistent
			Priority:        0,              // 0-9
			ReplyTo:         "",             //TODO
			CorrelationId:   "",             //TODO
			Body:            []byte(cmd),
		},
	); err != nil {
		return log.Errorf("Exchange Publish: %s", err)
	}
}
