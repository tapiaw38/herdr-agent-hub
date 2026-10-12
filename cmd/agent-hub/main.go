package main

import (
	"fmt"
	"github.com/tapiaw38/herdr-agent-hub/internal/app"
	"os"
)

func main() {
	var err error
	if len(os.Args) > 1 && os.Args[1] == "focus" {
		err = app.FocusHub()
	} else if len(os.Args) > 1 && os.Args[1] == "open" {
		err = app.OpenHub()
	} else {
		err = app.Run()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "agent-hub:", err)
		os.Exit(1)
	}
}
