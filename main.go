package main

import (
	"fmt"
	"os"
)

func main() {
	storePath, err := defaultStorePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve store path: %v\n", err)
		os.Exit(1)
	}

	store := NewStore(storePath)
	state, err := store.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load state: %v\n", err)
		os.Exit(1)
	}

	app := NewApp(store, state, os.Stdin, os.Stdout)
	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "run app: %v\n", err)
		os.Exit(1)
	}
}
