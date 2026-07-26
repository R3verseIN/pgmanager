package main

import (
	"fmt"
	"os"
)

func main() {
	printBanner()

	cfg, err := runWizard()
	if err != nil {
		fmt.Fprintln(os.Stderr, errorStyle.Render(fmt.Sprintf("Error: %v", err)))
		os.Exit(1)
	}

	printConfig(cfg)
}
