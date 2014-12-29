package conf

import (
	"encoding/json"
	"os"
	"time"
)

type ConfigStruct struct {
	Comet          string `json:"comet"`
	Web            string `json:"web"`
	CommandTimeout int    `json:"command_timeout"`
	PushAPI        string `json:"pushapi"`

	AcceptTimeout    uint32 `json:"accept_timeout"`
	ReadTimeout      uint32 `json:"read_timeout"`
	WriteTimeout     uint32 `json:"write_timeout"`
	HeartbeatTimeout uint32 `json:"heartbeat_timeout"`
	MaxBodyLen       uint32 `json:"max_bodylen"`
	MaxClients       uint32 `json:"max_clients"`

	Rabbit struct {
		Enable       bool   `json:"enable"`
		Uri          string `json:"uri"`
		Exchange     string `json:"exchange"`
		ExchangeType string `json:"exchange_type"`
		Key          string `json:"key"`
		Reliable     bool   `json:"reliable"`
		QOS          int    `json:"qos"`
	} `json:"rabbit"`

	Redis struct {
		Server   string `json:"server"`
		Pass     string `json:"pass"`
		PoolSize int    `json:"poolsize"`
		Retry    int    `json:"retry"`
	} `json:"redis"`

	ZooKeeper struct {
		Enable  bool          `json:"enable"`
		Addr    string        `json:"addr"`
		Timeout time.Duration `json:"timeout"`
		Path    string        `json:"path"`
	} `json:"zookeeper"`

	Auth struct {
		Provider string `json:"provider"`
		LetvUrl  string `json:"letv_url"`
	} `json:"auth"`

	DevCenter string `json:"devcenter"`
    Notify struct {
        Addr string             `json:"addr"`
        SubUrl string           `json:"sub_url"`
        PushUrl string          `json:"push_url"`
        MaxNotices int          `json:"max_notices"`
        Timeout time.Duration   `json:"timeout"`
    }                           `json:"notify"`

	Sync struct {
		Url string		`json:"url"`
		Interval int	`json:"interval"`
	} `json:"sync"`
}

var (
	Config ConfigStruct
)

func LoadConfig(filename string) error {
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
