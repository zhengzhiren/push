package auth

import (
	"github.com/chenyf/push/conf"
)

type AuthChecker interface {
	Auth(token string) (bool, string)
}

var (
	Instance AuthChecker = nil
)

func NewInstance(config *conf.ConfigStruct) bool {
	if config.Auth.Provider == "letv" {
		Instance = newLetvAuth(config)
		return true
	}
	return false
}

