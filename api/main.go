package main

import (
	"flag"
	"os"
	"log"
	"os/signal"
	"syscall"
	"net/http"
	"fmt"
	"time"
	"sync"
	"encoding/json"
	"github.com/garyburd/redigo/redis"
	"github.com/streadway/amqp"
	"github.com/chenyf/push/api/zk"
)

const (
	TimeToLive = "ttl"
	RedisServer = "10.154.156.121:6380"
	RedisPasswd = "rpasswd"
	MsgID = "msgid"
	MaxMsgCount = 9223372036854775807
)

type Message struct {
	MsgId		int64		`json:"msgid"`
	AppId		string		`json:"appid"`
	CTime		int64		`json:"ctime"`
	Platform	string		`json:"platform"`
	MsgType		int		`json:"msg_type"`
	PushType	int		`json:"push_type"`
	Content		string		`json:"content"`
	Options		interface{}	`json:"options"`
}

type PResponse struct {
	ErrNo	int	`json:"errno"`
	ErrMsg	string	`json:"errmsg"`
}

type Producer struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	exchangeName string
	routingKey string
}

var (
	uri          = flag.String("uri", "amqp://guest:guest@10.154.156.121:5672/", "AMQP URI")
	exchangeName = flag.String("exchange", "test-exchange", "Durable AMQP exchange name")
	exchangeType = flag.String("exchange-type", "fanout", "Exchange type - direct|fanout|topic|x-custom")
	routingKey   = flag.String("key", "test-key", "AMQP routing key")
	reliable     = flag.Bool("reliable", true, "Wait for the publisher confirmation before exiting")

	zkAddr	     = flag.String("zkaddr", "10.154.156.121:2181", "zookeeper addrs")
	zkTimeout    = flag.Duration("zktimeout", 30, "zookeeper timeout")
	zkPath       = flag.String("zkpath", "/push", "zookeeper path")
)

var msgBox = make(chan Message, 10)

func checkMessage(m *Message) bool {
	ret := true
	if m.AppId == "" || m.Content == "" || m.MsgType == 0 || m.PushType == 0 {
		ret = false
	}
	return ret
}

func getComet(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method Not Allowed", 405)
		return
	}
	node := zk.GetComet()
	if node == nil {
		http.Error(w, "No active comet", 404)
		return
	}
	fmt.Fprintf(w, node.TcpAddr)
}

func postSendMsg(w http.ResponseWriter, r *http.Request) {
	var response PResponse
	msg := Message{}
	response.ErrNo = 0
	if r.Method != "POST" {
		response.ErrNo  = 1001
		response.ErrMsg = "must using 'POST' method\n"
		goto ERROR
	}

	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		response.ErrNo  = 1002
		response.ErrMsg = "invaild POST body"
		goto ERROR
	}
	if msg.CTime == 0 {
		msg.CTime = time.Now().Unix()
	}
	//log.Print(msg)

	if !checkMessage(&msg) {
		response.ErrNo  = 1003
		response.ErrMsg = "invaild Message"
		goto ERROR

	} else {
		response.ErrMsg = ""
		msgBox <- msg
	}

	ERROR:
		b, _ := json.Marshal(response)
		fmt.Fprintf(w, string(b))
}

func setMsgID(c redis.Conn) error {
	ret, err := c.Do("EXISTS", MsgID)
	if err != nil {
		log.Printf("failed to set MsgID", err)
		return err
	}

	if ret == 0 {
		if _, err := c.Do("SET", MsgID, 0); err != nil {
			log.Printf("failed to set MsgID:", err)
			return err
		}
	}
	return nil

}

func getMsgID(c redis.Conn) int64 {
	id, err := redis.Int64(c.Do("INCR", MsgID))
	if err != nil {
		log.Printf("failed to get MsgID", err)
		return 0
	} else {
		return id
	}
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
		return nil, fmt.Errorf("failed to dial mq: %s", err)
	}

	p.channel, err = p.conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("failed to get channel: %s", err)
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
		return nil, fmt.Errorf("failed to declare exchange: %s", err)
	}

	/*if reliable {
		if err = p.channel.Confirm(false); err != nil {
			return nil, fmt.Errorf("failed to put channel into confirm mode: %s", err)
		}
	}*/

	return p, nil	
}

func (p *Producer) publish(data []byte) error {
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
		return fmt.Errorf("Exchange Publish: %s", err)
	}
	
	return nil
}

func confirmOne(ack, nack chan uint64) {
	select {
		case tag := <-ack:
			log.Printf("confirmed delivery with delivery tag: %d", tag)
		case tag := <-nack:
			log.Printf("failed delivery of delivery tag: %d", tag)
	}
}

func main() {
	pool := &redis.Pool{
		MaxIdle: 1,
		IdleTimeout: 300 * time.Second,
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", RedisServer)
			if err != nil {
				log.Printf("faild to connect Redis:", err)
				return nil, err
			}
			if _, err := c.Do("AUTH", RedisPasswd); err != nil {
				c.Close()
				return nil, err
			}
			return c, err
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}
	setMsgID(pool.Get())

	producer, err := NewProducer(*uri, *exchangeName, *exchangeType, *routingKey, *reliable)
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}
	err = zk.InitZK(*zkAddr, (*zkTimeout)*time.Second, *zkPath)
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}
	waitGroup := &sync.WaitGroup{}
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		sig := <-c
		log.Printf("Received signal '%v', exiting\n", sig)
		close(msgBox)
		pool.Close()
		producer.channel.Close()
		producer.conn.Close()
		waitGroup.Done()
	}()

	waitGroup.Add(1)
	go func() {
		http.HandleFunc("/v1/push/message", postSendMsg)
		http.HandleFunc("/v1/comet", getComet)
		err := http.ListenAndServe("0.0.0.0:8080", nil)
		if err != nil {
			log.Printf("failed to http listen:", err)
		}
	}()

	for {
		select {
			case m, ok := <-msgBox:
				if !ok {
					os.Exit(0)
				}

				mid := getMsgID(pool.Get())
				if mid == 0 {
					log.Printf("invaild MsgID")
					continue
				}

				m.MsgId = mid
				log.Printf("msg [%v] %v", mid, m)

				v, err := json.Marshal(m)
				if err != nil {
					log.Printf("failed to encode with Msg:", err)
					continue
				}

				if _, err := pool.Get().Do("HSET", m.AppId, mid, v); err != nil {
					log.Printf("failed to put Msg into redis:", err)
					continue
				}
				if m.Options != nil {
					ttl, ok := m.Options.(map[string]interface{})[TimeToLive]
					if ok && int64(ttl.(float64)) > 0 {
						if _, err := pool.Get().Do("HSET", m.AppId + "_offline", fmt.Sprintf("%v_%v", mid, int64(ttl.(float64))+m.CTime), v); err != nil {
							log.Printf("failed to put offline Msg into redis:", err)
							continue
						}
					}
				}

				d := map[string]interface{}{
					"appid": m.AppId,
					"msgid": mid,
				}
				data, err := json.Marshal(d)
				if err != nil {
					log.Printf("failed to jsonencode with data:", err)
					continue
				}

				if err := producer.publish(data); err != nil {
					log.Printf("failed to publish data:", err)
					continue
				}
		}
	}

	waitGroup.Wait()
}
