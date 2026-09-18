// Command dominion-client-tui is the terminal client for a dominion portal.
//
// It is a remote client, like the AppImage: it prompts for a server address and
// PIN, lists the server's tmux sessions, and attaches to one over the portal's
// WebSocket PTY bridge. Selecting a session suspends the TUI and pipes the
// terminal to the session; Ctrl-] detaches without killing it.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/brewingshell/dominion/internal/tuiclient"
)

// version is overridden at build time with -ldflags "-X main.version=…".
var version = "dev"

func main() {
	var (
		server   = flag.String("server", "", "dominion server host[:port] (skips the connect screen)")
		scheme   = flag.String("scheme", "", "http or https (default: saved, else http)")
		insecure = flag.Bool("insecure", false, "skip TLS certificate verification (for a self-signed server)")
		showVer  = flag.Bool("version", false, "print the version and exit")
	)
	flag.Parse()

	if *showVer {
		fmt.Printf("dominion-client-tui %s\n", version)
		return
	}

	err := tuiclient.Run(tuiclient.UIOptions{
		Server:   *server,
		Scheme:   *scheme,
		Insecure: *insecure,
		Version:  version,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "dominion-client-tui: %v\n", err)
		os.Exit(1)
	}
}
