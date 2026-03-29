// Command agentpulse-cli is the AgentPulse CLI tool.
//
// Usage:
//
//	agentpulse-cli eval check [flags]
//
// Run agentpulse-cli --help for full flag documentation.
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 3 {
		printUsage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "eval":
		switch os.Args[2] {
		case "check":
			runEvalCheck(os.Args[3:])
		default:
			fmt.Fprintf(os.Stderr, "unknown eval subcommand: %s\n", os.Args[2])
			printUsage()
			os.Exit(2)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(2)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `agentpulse-cli — AgentPulse CLI

Commands:
  eval check   Check eval scores against a threshold (CI quality gate)

Run "agentpulse-cli eval check --help" for flags.`)
}
