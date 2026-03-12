// mock-dsl is a test helper that emits canned NDJSON events to stdout,
// simulating the golem-dsl binary for DSLRunner integration tests.
package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: mock-dsl <command>")
		os.Exit(1)
	}

	cmd := args[0]
	switch cmd {
	case "list":
		fmt.Println("mock-agent           Mock agent for testing [built-in]")

	case "run":
		agent := ""
		goal := ""
		for i, a := range args {
			switch a {
			case "run":
				if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
					agent = args[i+1]
				}
			case "--goal":
				if i+1 < len(args) {
					goal = args[i+1]
				}
			}
		}
		_ = goal

		// Check for special agent names that trigger different behaviors
		switch agent {
		case "halt-agent":
			fmt.Println(`{"type":"step-start","step":"plan","iteration":1,"agent":"halt-agent"}`)
			fmt.Println(`{"type":"error","step":"plan","error-type":"unrecoverable","iteration":1}`)
			fmt.Println(`{"type":"agent-done","agent":"halt-agent","outcome":"halted","total-steps":1}`)
		case "fail-agent":
			fmt.Fprintln(os.Stderr, "Unknown agent: fail-agent")
			os.Exit(1)
		default:
			fmt.Println(`{"type":"step-start","step":"plan","iteration":1,"agent":"` + agent + `"}`)
			fmt.Println(`{"type":"step-end","step":"plan","state-version":1}`)
			fmt.Println(`{"type":"step-start","step":"implement","iteration":2,"agent":"` + agent + `"}`)
			fmt.Println(`{"type":"step-end","step":"implement","state-version":2}`)
			fmt.Println(`{"type":"agent-done","agent":"` + agent + `","outcome":"complete","total-steps":2}`)
		}

	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		os.Exit(1)
	}
}
