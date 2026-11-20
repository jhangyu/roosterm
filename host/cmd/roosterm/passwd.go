package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"golang.org/x/term"

	"roosterm/host/internal/auth"
	"roosterm/host/internal/config"
)

func passwd(args []string, stdin *os.File, stdout, stderr io.Writer) int {
	path, code, ok := configPath("passwd", args, stderr)
	if !ok {
		return code
	}
	if err := setPassword(path, stdin, stderr, auth.DefaultCost); err != nil {
		fmt.Fprintf(stderr, "roosterm passwd: %v\n", err)
		return exitError
	}
	fmt.Fprintf(stdout, "wrote %s\n", path)
	return exitOK
}

// setPassword takes the hash cost so tests can use a cheap one.
func setPassword(path string, stdin *os.File, prompts io.Writer, cost auth.Cost) error {
	cfg, err := loadOrNew(path)
	if err != nil {
		return err
	}
	p := &prompter{r: bufio.NewReader(stdin), fd: int(stdin.Fd()), tty: term.IsTerminal(int(stdin.Fd())), out: prompts}
	user, err := p.line("username: ")
	if err != nil {
		return err
	}
	pw, err := p.secret("password: ")
	if err != nil {
		return err
	}
	again, err := p.secret("repeat password: ")
	if err != nil {
		return err
	}
	switch {
	case user == "":
		return errors.New("username: expected a non-empty value, got \"\"")
	case pw == "":
		return errors.New("password: expected a non-empty value, got \"\"")
	case pw != again:
		return errors.New("repeat password: expected the same password, got a different one")
	}
	salt, err := auth.NewSalt()
	if err != nil {
		return err
	}
	cfg.Username = user
	cfg.PasswordHash = auth.HashPassword(pw, salt, cost)
	return config.Save(path, cfg)
}

// loadOrNew loads an existing config or starts one with defaults and the current $SHELL.
func loadOrNew(path string) (config.Config, error) {
	_, err := os.Stat(path)
	if err == nil {
		return config.Load(path)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return config.Config{}, fmt.Errorf("config %s: %w", path, err)
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		return config.Config{}, errors.New("SHELL is empty; run this from your login shell")
	}
	return config.Config{Listen: config.DefaultListen, PingInterval: config.DefaultPingInterval,
		SessionTTL: config.DefaultSessionTTL, Shell: shell}, nil
}

type prompter struct {
	r   *bufio.Reader
	fd  int
	tty bool
	out io.Writer
}

func (p *prompter) line(prompt string) (string, error) {
	fmt.Fprint(p.out, prompt)
	s, err := p.r.ReadString('\n')
	if err != nil && !(errors.Is(err, io.EOF) && s != "") {
		return "", fmt.Errorf("read %s: %w", strings.TrimSuffix(prompt, ": "), err)
	}
	return strings.TrimRight(s, "\r\n"), nil
}

// secret reads without echo on a terminal; scripted input (a pipe) is read as lines.
func (p *prompter) secret(prompt string) (string, error) {
	if !p.tty {
		return p.line(prompt)
	}
	fmt.Fprint(p.out, prompt)
	b, err := term.ReadPassword(p.fd)
	fmt.Fprintln(p.out)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", strings.TrimSuffix(prompt, ": "), err)
	}
	return string(b), nil
}
