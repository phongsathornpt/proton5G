package repository

import "testing"

func TestStatusShape(t *testing.T) {
	st := Status()
	if _, ok := st["mbimcli_available"]; !ok {
		t.Fatal("missing mbimcli_available")
	}
	if _, ok := st["device_present"]; !ok {
		t.Fatal("missing device_present")
	}
}
