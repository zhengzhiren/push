package devcenter

import (
	"github.com/chenyf/push/conf"
	"testing"
)

var (
	config string = "../pushapi/conf/conf.json"
	sso_tk string = "1025229b2axpbZeLS38ZYPf6uCJm1igjP5wJV63m3T6lkREq7fm2kH9LLxUeSNprievm3B8lf3"
)

func TestGetDeviceList(t *testing.T) {
	err := conf.LoadConfig(config)
	if err != nil {
		t.Errorf("load config failed, %s", err)
	}
	devList, err := GetDeviceList(sso_tk, DEV_ROUTER)
	if err != nil {
		t.Errorf("%s", err)
	}
	if len(devList) == 0 {
		t.FailNow()
	}
}

func BenchmarkGetDeviceList(b *testing.B) {
	b.StopTimer()
	err := conf.LoadConfig(config)
	if err != nil {
		b.Errorf("load config failed, %s", err)
	}
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		_, err := GetDeviceList(sso_tk, DEV_ROUTER)
		if err != nil {
			b.Errorf("%s", err)
		}
	}
}
