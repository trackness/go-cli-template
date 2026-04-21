// Command go-cli-template is the entry point for the CLI binary. It
// constructs a background context, builds BuildInfo from the
// -ldflags-injected variables, invokes cli.Run, and exits with the
// returned code.
package main

import (
	"context"
	"os"

	"github.com/example/go-cli-template/internal/cli"
)

// Build-time variables populated via goreleaser's -ldflags:
//
//	-X main.version=<tag>
//	-X main.commit=<short sha>
//	-X main.date=<build time>
var (
	version = "dev"
	commit  = ""
	date    = ""
)

func main() {
	code := cli.Run(
		context.Background(),
		cli.BuildInfo{Version: version, Commit: commit, Date: date},
		os.Args[1:],
		os.Stdout,
		os.Stderr,
	)
	os.Exit(int(code))
}
