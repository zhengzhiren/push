package utils

import (
	"testing"
)

func TestSign(t *testing.T) {
	key := "appsec_ckeasUHYFkAvEitqagAr"
	date := "Tue, 25 Nov 2014 14:00:52 CST"
	body := []byte("{\"content\":\"just a test\",\"msg_type\":1,\"push_type\":1}")

	s := Sign(key, "POST", body, date, nil)
	t.Logf("%s", s)

	if s != "38bfccc84fd44d0cb7181eea507f7cdf02959449" {
		t.Errorf("failed, %s", s)
	}

	params := map[string][]string{
		"k2": {"v2"},
		"k4": {"v4"},
		"k1": {"v1"},
		"k3": {"v31", "v33", "v32"},
	}

	s = Sign(key, "POST", body, date, params)
	t.Logf("%s", s)

	if s != "d5ad3cbcffd2efb8e21823c21f2c0eec3eb340d4" {
		t.Errorf("failed, %s", s)
	}
}
