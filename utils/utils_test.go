package utils

import (
	"testing"
)

func TestSign(t *testing.T) {
	key := "appsec_ckeasUHYFkAvEitqagAr"
	date := "Tue, 25 Nov 2014 14:00:52 CST"
	path := "/api/v1/message"
	body := []byte("{\"content\":\"just a test\",\"msg_type\":1,\"push_type\":1}")

	s := Sign(key, "POST", path, body, date, nil)
	t.Logf("signature: %s", s)

	if s != "3b635f825d3c34eb6497b636e35e81777ef3c659" {
		t.Errorf("failed, %s", s)
	}

	params := map[string][]string{
		"k2": {"v2"},
		"k4": {"v4"},
		"k1": {"v1"},
		"k3": {"v31", "v33", "v32"},
	}

	s = Sign(key, "POST", path, body, date, params)
	t.Logf("signature: %s", s)

	if s != "1a5b33a2aa4d3435d388142109629e393e513a81" {
		t.Errorf("failed, %s", s)
	}
}
