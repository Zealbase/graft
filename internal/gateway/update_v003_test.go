package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

func TestNewerVersion(t *testing.T) {
	cases := []struct {
		cur, latest string
		want        bool
	}{
		{"dev", "v0.0.3", true},
		{"", "v0.0.1", true},
		{"v0.0.2", "v0.0.3", true},
		{"0.0.2", "0.0.3", true},
		{"v0.0.3", "v0.0.3", false},
		{"v0.1.0", "v0.0.9", false},
		{"v1.0.0", "v0.9.9", false},
	}
	for _, c := range cases {
		if got := newerVersion(c.cur, c.latest); got != c.want {
			t.Errorf("newerVersion(%q,%q)=%v want %v", c.cur, c.latest, got, c.want)
		}
	}
}

// TestRunUpdateCheckOnly stubs the GitHub releases endpoint and verifies the
// check-only path reports the available update without invoking go install.
func TestRunUpdateCheckOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9"}`))
	}))
	defer srv.Close()

	oldAPI, oldClient, oldVer := releasesAPIFor(), updateHTTPClient, buildVersion
	t.Cleanup(func() { setReleasesAPI(oldAPI); updateHTTPClient = oldClient; buildVersion = oldVer })

	setReleasesAPI(srv.URL)
	updateHTTPClient = srv.Client()
	SetVersion("v0.0.1")

	res, err := RunUpdate(contract.UpdateOpts{CheckOnly: true})
	if err != nil {
		t.Fatalf("RunUpdate: %v", err)
	}
	if res.Current != "v0.0.1" || res.Latest != "v9.9.9" {
		t.Fatalf("unexpected versions: %+v", res)
	}
	if res.Updated {
		t.Fatalf("check-only must not set Updated")
	}
	if res.Notes == "" {
		t.Fatalf("expected an update-available note")
	}
}

// TestRunUpdateUpToDate verifies the no-op path when current >= latest.
func TestRunUpdateUpToDate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v0.0.1"}`))
	}))
	defer srv.Close()

	oldAPI, oldClient, oldVer := releasesAPIFor(), updateHTTPClient, buildVersion
	t.Cleanup(func() { setReleasesAPI(oldAPI); updateHTTPClient = oldClient; buildVersion = oldVer })

	setReleasesAPI(srv.URL)
	updateHTTPClient = srv.Client()
	SetVersion("v0.0.1")

	res, err := RunUpdate(contract.UpdateOpts{CheckOnly: true})
	if err != nil {
		t.Fatalf("RunUpdate: %v", err)
	}
	if res.Updated {
		t.Fatalf("up-to-date must not set Updated")
	}
}
