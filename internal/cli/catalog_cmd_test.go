package cli_test

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestCLICatalogVerifyOK: the embedded catalog verifies clean and exits 0.
func TestCLICatalogVerifyOK(t *testing.T) {
	out, err := execNoGate(t, nil, "catalog", "verify")
	if err != nil {
		t.Fatalf("catalog verify should exit 0: %v\n%s", err, out)
	}
	if !strings.Contains(out, "catalog OK") {
		t.Fatalf("expected 'catalog OK' line:\n%s", out)
	}
}

// TestCLICatalogVerifyJSON: -o json reports ok=true and the verified providers.
func TestCLICatalogVerifyJSON(t *testing.T) {
	out, err := execNoGate(t, nil, "catalog", "verify", "-o", "json")
	if err != nil {
		t.Fatalf("catalog verify json: %v\n%s", err, out)
	}
	var res struct {
		OK        bool     `json:"ok"`
		Verified  []string `json:"verified"`
		Providers int      `json:"providers"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("parse json: %v\n%s", err, out)
	}
	if !res.OK || res.Providers != 10 || len(res.Verified) != 10 {
		t.Fatalf("unexpected verify result: %+v", res)
	}
}
