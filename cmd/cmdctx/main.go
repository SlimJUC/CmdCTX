// cmdctx — Local AI-powered terminal assistant for safe command generation.
// This is the binary entry point. All logic lives in internal packages.
package main

import (
	"fmt"
	"os"

	"github.com/slim/cmdctx/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
