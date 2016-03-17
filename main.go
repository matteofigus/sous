package main

import (
	"fmt"
	"os"

	"github.com/opentable/sous2/cli"
)

func main() {

	defer handlePanic()

	// Create the dependency graph.
	g, err := cli.BuildGraph()
	if err != nil {
		die(err)
	}

	// Create a CLI
	c := &cli.CLI{
		OutWriter: os.Stdout,
		ErrWriter: os.Stderr,
		Env:       map[string]string{},
		Hooks: cli.Hooks{
			PreExecute: func(c cli.Command) error { return g.Inject(c) },
		},
	}

	// Create a new Sous command
	s := &cli.Sous{Version: Version}

	// Invoke Sous command
	c.Invoke(s, os.Args)

	// The CLI itself should manage exiting cleanly. If it fails to exit due to
	// so, that's due to programmer error, let the user know.
	die("error: sous did not exit correctly; please let the maintainers know")
}

func die(v ...interface{}) {
	fmt.Fprintln(os.Stderr, v...)
	os.Exit(70)
}

// handlePanic gives us one last chance to send a message to the user in case a
// panic leaks right up to the top of the program.
//
// To see the real panic message, disable panic handling by setting DEBUG=YES.
func handlePanic() {
	if os.Getenv("DEBUG") == "YES" {
		return
	}
	if r := recover(); r != nil {
		fmt.Println(panicMessage)
		fmt.Printf("Sous Version: %s\n\n", Version)
		panic(r)
	}
}

const panicMessage = `
################################################################################
#                                                                              #
#                                       OOPS                                   #
#                                                                              #
#        Sous has panicked, due to programmer error. Please report this        #
#        to the project maintainers at:                                        #
#                                                                              #
#                https://github.com/opentable/sous/issues                      #
#                                                                              #
#        Please include this entire message and the stack trace below          # 
#        and we will investigate and fix it as soon as possible.               #
#                                                                              #
#        Thanks for your help in improving Sous for all!                       #
#                                                                              #
#        - The OpenTable DevTools Team                                         #
#                                                                              #
################################################################################
`
