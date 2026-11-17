package main

import (
	"os"

	"github.com/sylphe88/secompose/internal/runner"
)

func main() {
	os.Exit(runner.Run(os.Args[1:]))
}
