// Command roosterm serves browser terminals backed by herdr-core.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"roosterm/host/internal/config"
)

const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

const usage = `usage:
  roosterm passwd [--config PATH]   set the login user and password
  roosterm serve  [--config PATH]   run the HTTP server
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin *os.File, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return exitUsage
	}
	switch args[0] {
	case "passwd":
		return passwd(args[1:], stdin, stdout, stderr)
	case "serve":
		return serve(args[1:], stdout, stderr)
	}
	fmt.Fprintf(stderr, "roosterm: unknown command %q\n%s", args[0], usage)
	return exitUsage
}

// configPath parses --config for one subcommand; ok false means exit with code.
func configPath(name string, args []string, stderr io.Writer) (path string, code int, ok bool) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	p := fs.String("config", "", "config file (default $XDG_CONFIG_HOME or ~/.config, then roosterm/config.toml)")
	if err := fs.Parse(args); err != nil {
		return "", exitUsage, false
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "roosterm %s: unexpected arguments %v\n%s", name, fs.Args(), usage)
		return "", exitUsage, false
	}
	if *p != "" {
		return *p, exitOK, true
	}
	d, err := config.DefaultPath()
	if err != nil {
		fmt.Fprintf(stderr, "roosterm %s: %v\n", name, err)
		return "", exitError, false
	}
	return d, exitOK, true
}
