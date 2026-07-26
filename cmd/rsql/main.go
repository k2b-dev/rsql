package main

import (
	"os"

	"github.com/k2b-dev/rsql/internal/cli"
)

var executeRoot = func() error {
	return cli.NewRootCmd().Execute()
}

func run() int {
	if err := executeRoot(); err != nil {
		return 1
	}
	return 0
}

func main() {
	os.Exit(run())
}
