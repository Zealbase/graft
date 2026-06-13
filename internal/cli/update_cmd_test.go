package cli_test

import (
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/cli"
)

// TestCLIUpdateRegisteredWithoutGate: `graft update` is constructed and runnable
// even when the gateway is nil (it calls gateway.RunUpdate directly, needing no
// workspace). We assert the command exists; the network call itself is covered
// by the gateway test. A network failure surfaces as a non-nil error, which is
// acceptable here — we only assert the command is wired and reachable.
func TestCLIUpdateRegisteredWithoutGate(t *testing.T) {
	c := cli.EntrypointWithVersion(nil, nil, "test")
	root := c.Root()
	var found bool
	for _, cmd := range root.Commands() {
		if cmd.Name() == "update" {
			found = true
			if cmd.Flags().Lookup("check") == nil {
				t.Fatalf("update command missing --check flag")
			}
		}
	}
	if !found {
		t.Fatalf("update command not registered on root")
	}
}

// TestCLIUpdateHelp confirms the help text renders without a gateway.
func TestCLIUpdateHelp(t *testing.T) {
	c := cli.EntrypointWithVersion(nil, nil, "test")
	root := c.Root()
	root.SetArgs([]string{"update", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("update --help: %v", err)
	}
}
