package conf

import (
	"os"
	"encoding/json"
)

type ConfigStruct struct {
	Comet string		`json:"comet"`
	Web string			`json:"web"`

	Rabbit struct {
		Uri string				`json:"uri"`
		Exchange string			`json:"exchange"`
		ExchangeType string		`json:"exchange-type"`
		Queue string			`json:"queue"`
		Key string				`json:"key"`
		ConsumerTag string		`json:"consumer-tag"`
		QOS int					`json:"qos"`
	}					`json:"rabbit"`

	Redis struct {
		Server string		`json:"server"`
		Pass string			`json:"pass"`
	}					`json:"redis"`
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

