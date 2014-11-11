package main

import (
	log "github.com/cihub/seelog"
	"github.com/streadway/amqp"
)

var (
	amqpURI      string = "amqp://guest:guest@10.154.156.121:5672/"
	exchange     string = "gibbon_rpc_exchange"
	exchangeType string = "fanout"
)

func StartRpcServer() error {
	conn, err := amqp.Dial(amqpURI)
	if err != nil {
		log.Errorf("Dial: %s", err)
		return err
	}
	//	defer conn.Close()

	log.Infof("got Connection, getting Channel")
	channel, err := conn.Channel()
	if err != nil {
		log.Errorf("Channel: %s", err)
		return err
	}

	log.Infof("got Channel, declaring %q Exchange (%q)", exchangeType, exchange)

	if err := channel.ExchangeDeclare(
		exchange,     // name
		exchangeType, // type
		true,         // durable
		false,        // auto-deleted
		false,        // internal
		false,        // noWait
		nil,          // arguments
	); err != nil {
		log.Errorf("Exchange Declare: %s", err)
		return err
	}

	rpcQueue, err := channel.QueueDeclare(
		"",    // name
		false, // durable
		true,  // autoDelete
		true,  // exclusive
		false, // noWait
		nil,   // args
	)
	if err != nil {
		log.Errorf("Queue Declare: %s", err)
	}
	log.Infof("declared RPC queue [%s]", rpcQueue.Name)

	if err = channel.QueueBind(
		rpcQueue.Name, // name of the queue
		"",            // bindingKey
		exchange,      // sourceExchange
		false,         // noWait
		nil,           // arguments
	); err != nil {
		return err
	}

	deliveries, err := channel.Consume(
		rpcQueue.Name, // name
		"",            // consumerTag,
		false,         // noAck
		false,         // exclusive
		false,         // noLocal
		false,         // noWait
		nil,           // arguments
	)
	if err != nil {
		log.Errorf("consume error: %s", err)
		return err
	}

	done := make(chan error)
	go handleRPC(deliveries, channel, done)

	return nil
}

func handleRPC(deliveries <-chan amqp.Delivery, channel *amqp.Channel, done chan error) {
	for d := range deliveries {
		log.Debugf(
			"got %dB RPC request [%s]: [%v] %q",
			len(d.Body),
			d.CorrelationId,
			d.DeliveryTag,
			d.Body,
		)
		d.Ack(false)

		if err := channel.Publish(
			"",        // publish to an exchange
			d.ReplyTo, // routingKey
			false,     // mandatory
			false,     // immediate
			amqp.Publishing{
				Headers:         amqp.Table{},
				ContentType:     "text/plain",
				ContentEncoding: "",
				DeliveryMode:    amqp.Transient, // 1=non-persistent, 2=persistent
				Priority:        0,              // 0-9
				ReplyTo:         "",
				CorrelationId:   d.CorrelationId, //TODO
				Body:            []byte("RPC response!"),
			},
		); err != nil {
			log.Errorf("Exchange Publish: %s", err)
		}
	}
	log.Infof("handle: deliveries channel closed")
	done <- nil
}
