package main

import (
	"os"

	taskbench "github.com/m2tkl/taskbench/src"
)

func main() {
	os.Exit(taskbench.Run(os.Args))
}
