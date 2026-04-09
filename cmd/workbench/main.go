package main

import (
	"os"

	workbench "github.com/m2tkl/workbench/src"
)

func main() {
	os.Exit(workbench.Run(os.Args))
}
