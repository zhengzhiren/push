package storage

import (
	"testing"
)

func TestHashGet(t *testing.T) {
	ins := newRedisStorage2("10.135.28.70:6379", "rpasswd")
	b, _ := ins.HashGet("db_xxx", "lala")
	if b != nil {
		t.Errorf("failed")
	}
}

