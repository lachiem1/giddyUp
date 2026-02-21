package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/lachiem1/giddyUp/internal/auth"
	"github.com/lachiem1/giddyUp/internal/upapi"
	"golang.org/x/term"
)

func main() {
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "auth":
			if len(os.Args) >= 3 && os.Args[2] == "set" {
				if err := runAuthSet(); err != nil {
					fmt.Fprintf(os.Stderr, "auth set error: %v\n", err)
					os.Exit(1)
				}
				fmt.Println("PAT saved to your system credential store.")
				return
			}
			fmt.Fprintln(os.Stderr, "usage: giddyup auth set")
			os.Exit(1)
		case "ping":
			if err := runPing(); err != nil {
				fmt.Fprintf(os.Stderr, "ping error: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("connected successfully")
			return
		}
	}

	pat, err := auth.LoadPAT()
	if err != nil {
		fmt.Fprintf(os.Stderr, "auth setup error: %v\n", err)
		os.Exit(1)
	}

	// Do not print token value.
	fmt.Printf("PAT loaded successfully (%d chars).\n", len(pat))
}

func runPing() error {
	if len(os.Args) != 2 {
		return errors.New("usage: giddyup ping")
	}

	pat, err := auth.LoadPAT()
	if err != nil {
		return err
	}

	client := upapi.New(pat)
	return client.Ping(context.Background())
}

func runAuthSet() error {
	if len(os.Args) != 3 {
		return errors.New("usage: giddyup auth set")
	}

	fmt.Print("Enter Up PAT: ")
	pat, err := readSecret()
	if err != nil {
		return err
	}
	fmt.Println()

	if strings.TrimSpace(pat) == "" {
		return errors.New("empty PAT")
	}

	return auth.SavePAT(pat)
}

func readSecret() (string, error) {
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		value, err := term.ReadPassword(fd)
		if err != nil {
			return "", err
		}
		return string(value), nil
	}

	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		if len(line) == 0 {
			return "", err
		}
	}
	return strings.TrimSpace(line), nil
}
