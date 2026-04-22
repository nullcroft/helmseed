package main

import (
	"os"

	"github.com/nullcroft/helmseed/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}