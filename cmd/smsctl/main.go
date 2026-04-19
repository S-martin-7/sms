package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "migrate":
		fmt.Fprintln(os.Stderr, "smsctl migrate: not yet implemented (plan task 7)")
		os.Exit(1)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: smsctl <command>")
	fmt.Fprintln(os.Stderr, "commands: migrate")
}
