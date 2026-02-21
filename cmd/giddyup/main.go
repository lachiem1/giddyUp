package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/lachiem1/giddyUp/internal/auth"
	"golang.org/x/term"
)

func main() {
	if len(os.Args) >= 3 && os.Args[1] == "auth" && os.Args[2] == "set" {
		if err := runAuthSet(); err != nil {
			fmt.Fprintf(os.Stderr, "auth set error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("PAT saved to your system credential store.")
		return
	}

	pat, err := auth.LoadPAT()
	if err != nil {
		fmt.Fprintf(os.Stderr, "auth setup error: %v\n", err)
		os.Exit(1)
	}

	// Do not print token value.
	fmt.Printf("PAT loaded successfully (%d chars).\n", len(pat))
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
