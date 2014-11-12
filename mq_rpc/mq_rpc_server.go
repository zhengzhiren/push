package mq_rpc

import (
	"encoding/json"
	"time"

	"github.com/chenyf/push/comet"
	log "github.com/cihub/seelog"
	"github.com/streadway/amqp"
)

type RpcServer struct {
	conn     *amqp.Connection
	channel  *amqp.Channel
	exchange string
}

func NewRpcServer(amqpURI, exchange string) (*RpcServer, error) {
	server := &RpcServer{
		exchange: exchange,
	}

	var err error
	server.conn, err = amqp.Dial(amqpURI)
	if err != nil {
		log.Errorf("Dial: %s", err)
		return nil, err
	}

	log.Infof("got Connection, getting Channel")
	server.channel, err = server.conn.Channel()
	if err != nil {
		log.Errorf("Channel: %s", err)
		return nil, err
	}

	log.Infof("got Channel, declaring %q Exchange (%q)", exchangeType, exchange)

	if err := server.channel.ExchangeDeclare(
		exchange,     // name
		exchangeType, // type
		true,         // durable
		false,        // auto-deleted
		false,        // internal
		false,        // noWait
		nil,          // arguments
	); err != nil {
		log.Errorf("Exchange Declare: %s", err)
		return nil, err
	}

	rpcQueue, err := server.channel.QueueDeclare(
		"",    // name
		false, // durable
		true,  // autoDelete
		true,  // exclusive
		false, // noWait
		nil,   // args
	)
	if err != nil {
		log.Errorf("Queue Declare: %s", err)
		return nil, err
	}
	log.Infof("declared RPC queue [%s]", rpcQueue.Name)

	if err = server.channel.QueueBind(
		rpcQueue.Name, // name of the queue
		"",            // bindingKey
		exchange,      // sourceExchange
		false,         // noWait
		nil,           // arguments
	); err != nil {
		log.Errorf("Queue bind: %s", err)
		return nil, err
	}

	deliveries, err := server.channel.Consume(
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
		return nil, err
	}

	go server.handleRpcRequest(deliveries)

	return server, nil
}

func (this *RpcServer) Stop() {
	this.conn.Close()
}

func (this *RpcServer) SendRpcResponse(callbackQueue, correlationId, result string) {
	if err := this.channel.Publish(
		"",            // publish to an exchange
		callbackQueue, // routingKey
		false,         // mandatory
		false,         // immediate
		amqp.Publishing{
			Headers:         amqp.Table{},
			ContentType:     "text/plain",
			ContentEncoding: "",
			DeliveryMode:    amqp.Transient, // 1=non-persistent, 2=persistent
			Priority:        0,              // 0-9
			ReplyTo:         "",
			CorrelationId:   correlationId,
			Body:            []byte("RPC response: " + result),
		},
	); err != nil {
		log.Errorf("Exchange Publish: %s", err)
	}
}

func (this *RpcServer) handleRpcRequest(deliveries <-chan amqp.Delivery) {
	for d := range deliveries {
		log.Debugf(
			"got %dB RPC request [%s]: [%v] %q",
			len(d.Body),
			d.CorrelationId,
			d.DeliveryTag,
			d.Body,
		)
		d.Ack(false)

		var msg MQ_CRTL_MSG
		if err := json.Unmarshal(d.Body, &msg); err != nil {
			log.Errorf("Unknown MQ message: %s", err)
			continue
		}

		cmdMsg := comet.CommandMessage{
			Service: msg.Service,
			Cmd:     msg.Cmd,
		}
		go func() {
			c := comet.DevicesMap.Get(msg.DeviceId)
			if c == nil {
				return
			}
			client := c.(*comet.Client)
			var replyChannel chan *comet.Message = nil
			wait := 10
			replyChannel = make(chan *comet.Message)
			bCmd, _ := json.Marshal(cmdMsg)
			seq, ok := client.SendMessage(comet.MSG_CMD, 0, bCmd, replyChannel)
			if !ok {
				return
			}
			select {
			case reply := <-replyChannel:
				var resp comet.CommandReplyMessage
				err := json.Unmarshal(reply.Data, &resp)
				if err != nil {
					log.Errorf("Bad command reply message: %s", err)
				}
				this.SendRpcResponse(d.ReplyTo, d.CorrelationId, resp.Result)
				return
			case <-time.After(time.Duration(wait) * time.Second):
				client.MsgTimeout(seq)
				return
			}
		}()
		//go comet.SendCommand(msg.DeviceId, &cmdMsg, 0)
	}

	log.Infof("handle: deliveries channel closed")
	//done <- nil
}
