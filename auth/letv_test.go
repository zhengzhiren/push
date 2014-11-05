package auth

import (
	"testing"
	"github.com/chenyf/push/conf"
)

func TestFoo(t *testing.T) {
	if err := conf.LoadConfig("../pushd/conf/conf.json"); err != nil {
		t.Errorf("load config failed, %s", err)
	}
	ins := newLetvAuth(&conf.Config)
	ins.Auth("nosuchtoken")
	ins.Auth("")
}

