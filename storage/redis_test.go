package storage

import (
	"testing"
)

func TestHashGet(t *testing.T) {
	t.Log("test HashGet")
	ins := newRedisStorage("10.176.28.144:6379", "rpasswd", 10, 3, 10, 3, 10, 10, 10)
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

func TestSetScan(t *testing.T) {
	t.Log("test SetScan")
	ins := newRedisStorage("10.176.28.144:6379", "rpasswd", 10, 3, 10, 3, 10, 10, 10)
	next, items, err := ins.SetScan("cyf", 0, 100)
	if err != nil {
		t.Logf("failed, %s\n", err)
		return
	}
	t.Logf("next: %d\n", next)
	for _, s := range items {
		t.Logf("%s\n", s)
	}
}
