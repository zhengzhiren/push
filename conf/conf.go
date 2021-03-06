package conf

import (
	"encoding/json"
	"os"
	"time"
)

type ConfigStruct struct {
	Comet struct {
		Port              string `json:"port"`
		AcceptTimeout     uint32 `json:"accept_timeout"`
		ReadTimeout       uint32 `json:"read_timeout"`
		WriteTimeout      uint32 `json:"write_timeout"`
		HeartbeatTimeout  uint32 `json:"heartbeat_timeout"`
		MaxBodyLen        uint32 `json:"max_bodylen"`
		MaxClients        uint32 `json:"max_clients"`
		WorkerCnt         int    `json:"worker_count"`
		HeartbeatInterval uint32 `json:"heartbeat_interval"`
		ReconnTime        uint32 `json:"reconn_time"`
	} `json:"comet"`
	Prof    bool   `json:"prof"`
	PushAPI string `json:"pushapi"`
	Web     string `json:"web"`

	Control struct {
		CommandTimeout int    `json:"command_timeout"`
		DevCenter      string `json:"devcenter"`
	} `json:"control"`

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
		Server       string `json:"server"`
		Pass         string `json:"pass"`
		MaxActive    int    `json:"maxactive"`
		MaxIdle      int    `json:"maxidle"`
		IdleTimeout  int    `json:"idletimeout"`
		Retry        int    `json:"retry"`
		ConnTimeout  int    `json:"conntimeout"`
		ReadTimeout  int    `json:"readtimeout"`
		WriteTimeout int    `json:"writetimeout"`
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

	Notify struct {
		Addr       string        `json:"addr"`
		SubUrl     string        `json:"sub_url"`
		PushUrl    string        `json:"push_url"`
		MaxNotices int           `json:"max_notices"`
		Timeout    time.Duration `json:"timeout"`
	} `json:"notify"`

	Sync struct {
		Url      string        `json:"url"`
		Interval time.Duration `json:"interval"`
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
