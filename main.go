package main

import (
	"os"

	"github.com/nullcroft/helmseed/cmd"
)

func run() error {
	return cmd.Execute()
}

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}
