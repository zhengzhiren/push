package conf

import (
	"os"
	"time"
	"encoding/json"
)

type ConfigStruct struct {
	Comet string		`json:"comet"`
	Web string			`json:"web"`
	PushAPI string		`json:"pushapi"`

	AcceptTimeout uint32	`json:"accept_timeout"`
	ReadTimeout uint32		`json:"read_timeout"`
	WriteTimeout uint32		`json:"write_timeout"`
	HeartbeatTimeout uint32	`json:"heartbeat_timeout"`

	Rabbit struct {
		Enable bool				`json:"enable"`
		Uri string				`json:"uri"`
		Exchange string			`json:"exchange"`
		ExchangeType string		`json:"exchange_type"`
		Key string				`json:"key"`
		Reliable bool			`json:"reliable"`
		QOS int					`json:"qos"`
	}					`json:"rabbit"`

	Redis struct {
		Server string		`json:"server"`
		Pass string		`json:"pass"`
	}				`json:"redis"`

	ZooKeeper struct {
		Enable bool				`json:"enable"`
		Addr string				`json:"addr"`
		Timeout time.Duration	`json:"timeout"`
		Path string				`json:"path"`
	}				`json:"zookeeper"`

	Auth struct {
		Provider string			`json:"provider"`
		LetvUrl string			`json:"letv_url"`
	}				`json:"auth"`
}

var (
	Config ConfigStruct
)

func LoadConfig(filename string) (error) {
	r, err := os.Open(filename)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(r)
	err = decoder.Decode(&Config)
	if err != nil {
		return err
	}
	return nil
}

