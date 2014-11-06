package storage

import (
	"testing"
)

func TestHashGet(t *testing.T) {
	ins := newRedisStorage2("10.135.28.70:6379", "rpasswd")
	b, err := ins.HashGet("db_app_8c02385507b349679307e355e7491636", "b814c6675d902816666ab4954b3ff40b3da7aa27")
	if err != nil {
		t.Log("failed, %s", err)
	}
	if b == nil {
		t.Log("empty")
	} else {
		t.Log("%s", string(b))
	}
}

