package conf

import (
	"os"
	"encoding/json"
)

type ConfigStruct struct {
	Comet string		`json:"comet"`
	Web string		`json:"web"`

	Rabbit struct {
		Uri string			`json:"uri"`
		Exchange string			`json:"exchange"`
		QOS int				`json:"qos"`
	}					`json:"rabbit"`

	Redis struct {
		Server string		`json:"server"`
		Pass string		`json:"pass"`
	}				`json:"redis"`
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

