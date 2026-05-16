package main

import (
	"os"

	"github.com/ujjalsharma100/lockie/internal/cli"
)

func main() {
	if err := cli.NewRoot().Execute(); err != nil {
		os.Exit(1)
	}
}
