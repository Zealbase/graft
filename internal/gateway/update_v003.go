package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

// releasesAPI is the GitHub "latest release" endpoint. It is a var (not a const)
// so tests can point it at a local stub server.
var releasesAPI = "https://api.github.com/repos/Shaik-Sirajuddin/graft/releases/latest"

// releasesAPIFor / setReleasesAPI are test seams for swapping the endpoint.
func releasesAPIFor() string    { return releasesAPI }
func setReleasesAPI(url string) { releasesAPI = url }

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
	// Requires the Go toolchain on PATH; surface a clear pointer otherwise.
	if _, lerr := exec.LookPath("go"); lerr != nil {
		return res, fmt.Errorf("gateway: update needs the Go toolchain on PATH (`go` not found); "+
			"install Go or grab a binary from https://github.com/Shaik-Sirajuddin/graft/releases")
	}
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
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, releasesAPI, nil)
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
	// Bound the response body: the release API returns a fixed-shape JSON object;
	// 64 KiB is far more than the tag_name we read and caps a hostile/huge body.
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&body); err != nil {
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

// cmpSemver compares two dotted numeric version strings; returns >0 if a>b. A
// pre-release suffix (e.g. "3-rc1") is split off each component before the
// numeric compare so it never corrupts the release number. When the numeric
// parts tie, a version carrying a pre-release suffix ranks BELOW the equivalent
// final release (per SemVer: 0.0.3-rc1 < 0.0.3), so a user on an rc is offered
// the final. Pre-release identifiers themselves are not ordered against each
// other (rc1 vs rc2 compares equal) — that finer ordering is out of scope.
func cmpSemver(a, b string) int {
	aNum, aPre := splitPreRelease(a)
	bNum, bPre := splitPreRelease(b)
	as, bs := strings.Split(aNum, "."), strings.Split(bNum, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		var ai, bi int
		if i < len(as) {
			fmt.Sscanf(numericPrefix(as[i]), "%d", &ai)
		}
		if i < len(bs) {
			fmt.Sscanf(numericPrefix(bs[i]), "%d", &bi)
		}
		if ai != bi {
			if ai > bi {
				return 1
			}
			return -1
		}
	}
	// Numeric parts tie: a pre-release ranks below the final release.
	switch {
	case aPre && !bPre:
		return -1
	case !aPre && bPre:
		return 1
	default:
		return 0
	}
}

// splitPreRelease separates the numeric "x.y.z" portion from a SemVer
// pre-release/build suffix. It returns the numeric part and whether a
// pre-release suffix ("-...") was present. A build-metadata suffix ("+...") is
// dropped without counting as a pre-release.
func splitPreRelease(v string) (numeric string, isPre bool) {
	// Strip build metadata first ("+..."), then a pre-release ("-...").
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	if i := strings.IndexByte(v, '-'); i >= 0 {
		return v[:i], true
	}
	return v, false
}

// numericPrefix returns the leading digits of a version component, defensively
// stripping any stray non-numeric suffix.
func numericPrefix(s string) string {
	for i, r := range s {
		if r < '0' || r > '9' {
			return s[:i]
		}
	}
	return s
}
