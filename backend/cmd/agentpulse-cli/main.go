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
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "eval":
		if len(os.Args) < 3 {
			printUsage()
			os.Exit(2)
		}
		switch os.Args[2] {
		case "check":
			runEvalCheck(os.Args[3:])
		default:
			fmt.Fprintf(os.Stderr, "unknown eval subcommand: %s\n", os.Args[2])
			printUsage()
			os.Exit(2)
		}
	case "replay":
		runReplay(os.Args[2:])
	case "runs":
		if len(os.Args) < 3 {
			printUsage()
			os.Exit(2)
		}
		switch os.Args[2] {
		case "list":
			runRunsList(os.Args[3:])
		case "tail":
			runRunsTail(os.Args[3:])
		default:
			fmt.Fprintf(os.Stderr, "unknown runs subcommand: %s\n", os.Args[2])
			printUsage()
			os.Exit(2)
		}
	case "status":
		runStatus(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(2)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `agentpulse-cli — AgentPulse CLI

Commands:
  eval check         Check eval scores against a threshold (CI quality gate)
  replay <run-id>    Download a run's replay bundle for local sandbox debugging
  runs list          List recent runs for a project
  runs tail          Live tail of incoming spans for a project
  status             Check collector health (exits 0=healthy, 1=unhealthy)

Run "agentpulse-cli <command> --help" for flags.`)
}
