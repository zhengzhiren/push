package main

import (
	log "github.com/cihub/seelog"
	"github.com/streadway/amqp"
)

var (
	exchangeType string = "fanout"
	routingKey   string = ""
)

type RpcClient struct {
	conn          *amqp.Connection
	channel       *amqp.Channel
	exchange      string
	callbackQueue string
}

func (this *RpcClient) Close() {
	this.conn.Close()
}

var (
	rpcClient *RpcClient = nil
)

func init_rpc(amqpURI, exchange string) error {
	rpcClient = &RpcClient{
		exchange: exchange,
	}

	var err error
	rpcClient.conn, err = amqp.Dial(amqpURI)
	if err != nil {
		log.Errorf("Dial: %s", err)
		return err
	}

	log.Infof("got Connection, getting Channel")
	rpcClient.channel, err = rpcClient.conn.Channel()
	if err != nil {
		log.Errorf("Channel: %s", err)
		return err
	}

	log.Infof("got Channel, declaring %q Exchange (%q)", exchangeType, exchange)

	if err := rpcClient.channel.ExchangeDeclare(
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

	callbackQueue, err := rpcClient.channel.QueueDeclare(
		"",    // name
		false, // durable
		true,  // autoDelete
		true,  // exclusive
		false, // noWait
		nil,   // args
	)
	if err != nil {
		log.Errorf("callbackQueue Declare error: %s", err)
		return err
	}
	rpcClient.callbackQueue = callbackQueue.Name
	log.Infof("declared callback queue [%s]", rpcClient.callbackQueue)
	log.Infof("MQ RPC succesfully inited")

	go handleResponse()

	return nil
}

func handleResponse() {
	deliveries, err := rpcClient.channel.Consume(
		rpcClient.callbackQueue, // name
		"",    // consumerTag,
		false, // noAck
		false, // exclusive
		false, // noLocal
		false, // noWait
		nil,   // arguments
	)
	if err != nil {
		log.Errorf("consume error: %s", err)
		//return "", err
	}

	for d := range deliveries {
		log.Debugf(
			"got %dB response [%s]: [%v] %q",
			len(d.Body),
			d.CorrelationId,
			d.DeliveryTag,
			d.Body,
		)
		d.Ack(false)
	}
}

func control(deviceId string, service string, cmd string) (string, error) {
	log.Infof("publishing %dB cmd (%q)", len(cmd), cmd)
	if err := rpcClient.channel.Publish(
		rpcClient.exchange, // publish to an exchange
		routingKey,         // routing to 0 or more queues
		false,              // mandatory
		false,              // immediate
		amqp.Publishing{
			Headers:         amqp.Table{},
			ContentType:     "text/plain",
			ContentEncoding: "",
			DeliveryMode:    amqp.Transient, // 1=non-persistent, 2=persistent
			Priority:        0,              // 0-9
			ReplyTo:         rpcClient.callbackQueue,
			CorrelationId:   "XXX", //TODO
			Body:            []byte(cmd),
		},
	); err != nil {
		log.Errorf("Exchange Publish: %s", err)
		return "", err
	}

	return "", nil
}
