package main

import (
	"os"

	"dashboard.tw/backend/internal/cli"
)

var version = "dev"

func main() {
	if err := cli.NewRoot(version).Execute(); err != nil {
		os.Exit(1)
	}
}
