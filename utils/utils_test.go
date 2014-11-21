package utils

import (
	"testing"
)

func TestSign(t *testing.T) {
	d := map[string]string {
		"xxx" : "xxx",
		"yyy" : "yyy",
		"aaa" : "aaa",
	}

	s := Sign("GET", d, nil, "aaa")
	t.Logf("%s", s)

	/*if s != "aaa=aaaxxx=xxxyyy=yyy" {
		t.Errorf("failed, %s", s)
	}*/
}
