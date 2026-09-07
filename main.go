// Command mdq queries markdown files: outlines, single sections, section
// aware search and task lists, so an agent can read one part of a document
// instead of the whole file.
package main

import (
	"os"

	"github.com/jasperlyons/mdq/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
