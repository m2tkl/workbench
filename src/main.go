package main

import (
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	storePath, err := defaultStorePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve store path: %v\n", err)
		os.Exit(1)
	}

	store := NewStore(storePath)
	if len(os.Args) > 1 && os.Args[1] == "--seed-demo" {
		if err := store.Save(demoState(time.Now())); err != nil {
			fmt.Fprintf(os.Stderr, "seed demo data: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stdout, "demo data written to %s\n", store.TasksPath())
		return
	}

	state, err := store.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load state: %v\n", err)
		os.Exit(1)
	}

	program := tea.NewProgram(NewApp(store, state), tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "run app: %v\n", err)
		os.Exit(1)
	}
}
