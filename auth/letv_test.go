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
	ok, uid := ins.Auth("nosuchtoken")
	if ok {
		t.FailNow()
	}
	ok, uid = ins.Auth("102304f687BrX9DhNzo2LnEm1qjEpRrhhIqm1DqGyWbXQaEPUNMInqXcO7s2bChpFIeYz1Xq")
	if !ok {
		t.FailNow()
	}
	t.Logf("uid: %s", uid)
}

