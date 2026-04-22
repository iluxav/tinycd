package main

import (
	"fmt"
	"os"

	"github.com/iluxav/tinycd/internal/app"
)

var version = "dev"

func main() {
	root := app.NewRootCmd(version)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "tcd: "+err.Error())
		os.Exit(1)
	}
}
