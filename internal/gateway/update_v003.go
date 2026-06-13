package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// buildVersion is the running binary's version, recorded by the CLI from the
// ldflags-set main.Version. "dev" means an unversioned local build.
var buildVersion = "dev"

// SetVersion records the running binary's version for `graft update`.
func SetVersion(v string) {
	if strings.TrimSpace(v) != "" {
		buildVersion = v
	}
}

const releasesAPI = "https://api.github.com/repos/Shaik-Sirajuddin/graft/releases/latest"

// httpClient is the bounded client used for the release check.
var updateHTTPClient = &http.Client{Timeout: 10 * time.Second}

// RunUpdate performs the self-update check/apply WITHOUT needing a workspace or
// gateway (plan-sync task 6). The `update` CLI command calls this directly so it
// works outside an initialized repo.
func RunUpdate(opts contract.UpdateOpts) (contract.UpdateResult, error) {
	res := contract.UpdateResult{Current: buildVersion}
	latest, err := latestRelease()
	if err != nil {
		return res, fmt.Errorf("gateway: check latest release: %w", err)
	}
	res.Latest = latest

	if opts.CheckOnly {
		if newerVersion(buildVersion, latest) {
			res.Notes = fmt.Sprintf("update available: %s → %s (run `graft update`)", buildVersion, latest)
		} else {
			res.Notes = "up to date"
		}
		return res, nil
	}

	if !newerVersion(buildVersion, latest) {
		res.Notes = "already up to date"
		return res, nil
	}

	// Apply: `go install ...@latest` is the dependency-free mechanism. (A
	// release-asset self-replace via go-selfupdate is a planned enhancement.)
	cmd := exec.Command("go", "install", "github.com/Shaik-Sirajuddin/graft/cmd/graft@latest")
	if out, err := cmd.CombinedOutput(); err != nil {
		return res, fmt.Errorf("gateway: go install @latest failed: %v: %s", err, strings.TrimSpace(string(out)))
	}
	res.Updated = true
	res.Notes = fmt.Sprintf("updated %s → %s via `go install @latest`", buildVersion, latest)
	return res, nil
}

// Update satisfies contract.EntryGate by delegating to RunUpdate (it needs no
// gate state).
func (g *gate) Update(opts contract.UpdateOpts) (contract.UpdateResult, error) {
	return RunUpdate(opts)
}

// latestRelease fetches the latest published release tag from GitHub.
func latestRelease() (string, error) {
	req, err := http.NewRequest(http.MethodGet, releasesAPI, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := updateHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github releases API: status %d", resp.StatusCode)
	}
	var body struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	if body.TagName == "" {
		return "", fmt.Errorf("github releases API: empty tag_name")
	}
	return body.TagName, nil
}

// newerVersion reports whether latest is newer than current. A "dev" current is
// always considered older. Comparison is a lexical fallback on the
// dot-separated numeric components after stripping a leading "v".
func newerVersion(current, latest string) bool {
	if current == "dev" || current == "" {
		return true
	}
	return cmpSemver(strings.TrimPrefix(latest, "v"), strings.TrimPrefix(current, "v")) > 0
}

// cmpSemver compares two dotted numeric version strings; returns >0 if a>b.
func cmpSemver(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		var ai, bi int
		if i < len(as) {
			fmt.Sscanf(as[i], "%d", &ai)
		}
		if i < len(bs) {
			fmt.Sscanf(bs[i], "%d", &bi)
		}
		if ai != bi {
			if ai > bi {
				return 1
			}
			return -1
		}
	}
	return 0
}
