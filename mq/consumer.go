package mq

import (
	"fmt"
	"log"
	//"time"
	"encoding/json"
	"github.com/streadway/amqp"
	"github.com/chenyf/push/comet"
	"github.com/chenyf/push/storage"
)

type Consumer struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	queue   string
	tag     string
	done    chan error
}

func NewConsumer(amqpURI, exchange, exchangeType, queueName, key, ctag string, qos int) (*Consumer, error) {
	c := &Consumer{
		conn:    nil,
		channel: nil,
		queue:   queueName,
		tag:     ctag,
		done:    make(chan error),
	}

	var err error

	log.Printf("dialing %q", amqpURI)
	c.conn, err = amqp.Dial(amqpURI)
	if err != nil {
		return nil, fmt.Errorf("Dial: %s", err)
	}

	/*go func() {
		log.Printf("closing: %s", <-c.conn.NotifyClose(make(chan *amqp.Error)))
	}()*/

	log.Printf("got Connection, getting Channel")
	c.channel, err = c.conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("Channel: %s", err)
	}

	queue, err := c.channel.QueueDeclare(
		queueName, // name of the queue
		true,      // durable
		false,     // delete when usused
		false,     // exclusive
		false,     // noWait
		nil,       // arguments
	)
	if err != nil {
		return nil, fmt.Errorf("Queue Declare: %s", err)
	}

	log.Printf("declared Queue (%q %d messages, %d consumers), binding to Exchange (key %q)",
		queue.Name, queue.Messages, queue.Consumers, key)

	if err = c.channel.QueueBind(
		queue.Name, // name of the queue
		key,        // bindingKey
		exchange,   // sourceExchange
		false,      // noWait
		nil,        // arguments
	); err != nil {
		return nil, fmt.Errorf("Queue Bind: %s", err)
	}

	c.channel.Qos(qos, 0, false)
	return c, nil
}

func (c *Consumer) Consume() error {
	deliveries, err := c.channel.Consume(
		c.queue,    // name
		c.tag,      // consumerTag,
		false,      // noAck
		false,      // exclusive
		false,      // noLocal
		false,      // noWait
		nil,        // arguments
	)
	if err != nil {
		return fmt.Errorf("Queue Consume: %s", err)
	}

	go handle(deliveries, c.done)
	return nil
}

func (c *Consumer) Shutdown() error {
	// will close() the deliveries channel
	if err := c.channel.Cancel(c.tag, true); err != nil {
		return fmt.Errorf("Consumer cancel failed: %s", err)
	}

	if err := c.conn.Close(); err != nil {
		return fmt.Errorf("AMQP connection close error: %s", err)
	}

	defer log.Printf("AMQP shutdown OK")

	// wait for handle() to exit
	return <-c.done
}

func handle(deliveries <-chan amqp.Delivery, done chan error) {
	for d := range deliveries {
		log.Printf(
			"got %dB delivery: [%v] %q",
			len(d.Body),
			d.DeliveryTag,
			d.Body,
		)
		d.Ack(false)
		m := make(map[string]interface{})
		if err := json.Unmarshal(d.Body, &m); err != nil {
			log.Printf("failed to decode transMsg:", err)
			continue
		}
		log.Print(m, " ", m["msgid"])
		rawMsg := comet.RawMessage{}
		t := []byte(storage.StorageInstance.GetMsg(m["appid"].(string), int64(m["msgid"].(float64))))
		if err := json.Unmarshal(t, &rawMsg); err != nil {
			log.Printf("failed to decode rawMsg:", err)
			continue
		}
		log.Print(rawMsg)
		
		/*msg := comet.PushMessage{
			MsgId : 1000,
			AppId : appid,
			MsgType : 0,
			Payload : cmd,
		}
		b, _ := json.Marshal(msg)
		comet.SimplePushOutMessage(appid, 0, "", b)*/
	}
	log.Printf("handle: deliveries channel closed")
	done <- nil
}
