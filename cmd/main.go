package main

import (
	"fmt"
	"os"

	"plexmusic-tui/cmd/root"
)

func main() {
	if err := root.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
