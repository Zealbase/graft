// Command graft is the CLI entrypoint. It is intentionally thin: it fast-paths
// version, lazily constructs the gateway only for commands that need it, wires
// the CLI entrypoint, and runs it.
package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/Shaik-Sirajuddin/graft/internal/cli"
	"github.com/Shaik-Sirajuddin/graft/internal/cli/config"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/gateway"
)

// Version is overridden via -ldflags at build time.
var Version = "dev"

func main() {
	args := os.Args[1:]
	if printVersionOnly(args) {
		return
	}

	var gate contract.EntryGate
	if commandRequiresGateway(args) {
		root, err := os.Getwd()
		if err != nil {
			log.Fatalf("[ERROR] resolve working directory: %v", err)
		}
		gate, err = gateway.Open(root)
		if err != nil {
			log.Fatalf("[ERROR] %v", err)
		}
		defer gate.Close()
	}

	c := cli.EntrypointWithVersion(gate, &config.DefaultResolver{}, Version)
	if err := c.Install(); err != nil {
		log.Fatalf("[ERROR] %v", err)
	}
}

// commandRequiresGateway reports whether the first non-flag arg names a command
// that needs the gateway (store/engine). config, help, and version do not.
func commandRequiresGateway(args []string) bool {
	switch firstCommandArg(args) {
	case "init", "agent", "agents", "sync", "validate", "skill":
		return true
	default:
		return false
	}
}

func firstCommandArg(args []string) string {
	for _, a := range args {
		a = strings.TrimSpace(a)
		if a == "" || strings.HasPrefix(a, "-") {
			continue
		}
		return a
	}
	return ""
}

func printVersionOnly(args []string) bool {
	if len(args) != 1 {
		return false
	}
	switch args[0] {
	case "--version", "version":
		fmt.Println(Version)
		return true
	default:
		return false
	}
}
