package safemap

import (
	"testing"
)

func TestSafeMap(t *testing.T) {
	mp := NewSafeMap()
	if !mp.Set("lijie", int(1)) {
		t.Error("set error")
	}

	if !mp.Check("lijie") {
		t.Error("check error")
	}

	if v := mp.Get("lijie"); v.(int) != 1 {
		t.Error("get error")
	}

	if mp.Size() != 1 {
		t.Error("size error")
	}

	mp.Delete("lijie")
	if mp.Check("lijie") {
		t.Error("delete error")
	}

	mp.Delete("no")
}
