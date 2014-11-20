package mq

import (
	"encoding/json"
	"strconv"
	"time"
	//	"sync"
	"sync/atomic"

	log "github.com/cihub/seelog"
	"github.com/streadway/amqp"

	"github.com/chenyf/push/storage"
)

var (
	rpcExchangeType string = "direct"
)

type NoDeviceError struct {
	msg string
}

func (this *NoDeviceError) Error() string {
	return this.msg
}

type TimeoutError struct {
	msg string
}

func (this *TimeoutError) Error() string {
	return this.msg
}

type InvalidServiceError struct {
	msg string
}

func (this *InvalidServiceError) Error() string {
	return this.msg
}

type SdkError struct {
	msg string
}

func (this *SdkError) Error() string {
	return this.msg
}

type RpcClient struct {
	conn          *amqp.Connection
	channel       *amqp.Channel
	exchange      string
	callbackQueue string
	requestId     uint32
	requestTable  map[uint32]chan []byte
	rpcTimeout    int
}

func (this *RpcClient) Close() {
	this.conn.Close()
}

func (this *RpcClient) nextReqeustId() uint32 {
	return atomic.AddUint32(&this.requestId, 1)
}

func NewRpcClient(amqpURI, exchange string) (*RpcClient, error) {
	client := &RpcClient{
		exchange:     exchange,
		requestId:    0,
		rpcTimeout:   10,
		requestTable: make(map[uint32]chan []byte),
	}

	var err error
	client.conn, err = amqp.Dial(amqpURI)
	if err != nil {
		log.Errorf("Dial: %s", err)
		return nil, err
	}

	log.Infof("got Connection, getting Channel")
	client.channel, err = client.conn.Channel()
	if err != nil {
		log.Errorf("Channel: %s", err)
		return nil, err
	}

	log.Infof("got Channel, declaring %q Exchange (%q)", rpcExchangeType, exchange)

	if err := client.channel.ExchangeDeclare(
		exchange,        // name
		rpcExchangeType, // type
		true,            // durable
		false,           // auto-deleted
		false,           // internal
		false,           // noWait
		nil,             // arguments
	); err != nil {
		log.Errorf("Exchange Declare: %s", err)
		return nil, err
	}

	callbackQueue, err := client.channel.QueueDeclare(
		"",    // name
		false, // durable
		true,  // autoDelete
		true,  // exclusive
		false, // noWait
		nil,   // args
	)
	if err != nil {
		log.Errorf("callbackQueue Declare error: %s", err)
		return nil, err
	}
	client.callbackQueue = callbackQueue.Name
	log.Infof("declared callback queue [%s]", client.callbackQueue)
	log.Infof("MQ RPC succesfully inited")

	go handleResponse(client)

	return client, nil
}

func handleResponse(client *RpcClient) {
	deliveries, err := client.channel.Consume(
		client.callbackQueue, // name
		"",                   // consumerTag,
		false,                // noAck
		false,                // exclusive
		false,                // noLocal
		false,                // noWait
		nil,                  // arguments
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
		requestId, err := strconv.Atoi(d.CorrelationId)
		if err != nil {
			log.Errorf("Invalid RPC response Id: %s", d.CorrelationId)
			continue
		}
		replyCh, ok := client.requestTable[uint32(requestId)]
		if ok {
			replyCh <- d.Body
		} else {
			log.Warnf("Unknown RPC response Id: %d", requestId)
		}
	}
}

func (this *RpcClient) Control(deviceId string, service string, cmd string) (string, error) {
	serverName, err := storage.Instance.CheckDevice(deviceId)
	if err != nil {
		log.Errorf("failed to check device existence:", err)
		return "", err
	}
	if serverName == "" {
		return "", &NoDeviceError{"No device"}
	}

	requestId := this.nextReqeustId()
	msg := MQ_Msg_Crtl{
		DeviceId: deviceId,
		Service:  service,
		Cmd:      cmd,
	}

	msgData, _ := json.Marshal(&msg)

	log.Infof("publishing %dB cmd (%q)", len(cmd), cmd)
	if err := this.channel.Publish(
		this.exchange, // publish to an exchange
		serverName,    // routing to 0 or more queues
		false,         // mandatory
		false,         // immediate
		amqp.Publishing{
			Headers:         amqp.Table{},
			ContentType:     "text/plain",
			ContentEncoding: "",
			DeliveryMode:    amqp.Transient, // 1=non-persistent, 2=persistent
			Priority:        0,              // 0-9
			ReplyTo:         this.callbackQueue,
			CorrelationId:   strconv.Itoa(int(requestId)),
			Body:            msgData,
		},
	); err != nil {
		log.Errorf("Exchange Publish: %s", err)
		return "", err
	}

	//TODO: lock
	replyCh := make(chan []byte)
	this.requestTable[requestId] = replyCh
	defer delete(this.requestTable, requestId)
	defer close(replyCh)

	select {
	case replyData := <-replyCh:
		log.Infof("RPC response [%d]: %s", requestId, replyData)
		var reply MQ_Msg_CtrlReply
		if err := json.Unmarshal(replyData, &reply); err != nil {
			log.Errorf("failed to unmarshal RPC reply: %s", err)
			return "", err
		}
		switch reply.Status {
		case 1:
			return "", &InvalidServiceError{"Invalid service name"}
		case 2:
			return "", &SdkError{"Exception on calling service"}
		}
		return reply.Result, nil
	case <-time.After(time.Duration(this.rpcTimeout) * time.Second):
		log.Warnf("RPC request [%d] timeout", requestId)
		return "", &TimeoutError{"RPC timeout"}
	}
}
