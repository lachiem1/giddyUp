package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lachiem1/giddyUp/internal/storage"
	"github.com/lachiem1/giddyUp/internal/tui"
)

func main() {
	if len(os.Args) >= 2 {
		fmt.Fprintln(os.Stderr, "CLI subcommands were removed. Launch giddyup with no args and use slash commands in the TUI (for example: /connect, /ping, /db-wipe).")
		os.Exit(1)
	}

	if _, _, err := initDB(); err != nil {
		fmt.Fprintf(os.Stderr, "db setup error: %v\n", err)
		os.Exit(1)
	}

	if err := runTUI(); err != nil {
		fmt.Fprintf(os.Stderr, "tui error: %v\n", err)
		os.Exit(1)
	}
}

func initDB() (*sql.DB, storage.Config, error) {
	return storage.Open(context.Background())
}

func runTUI() error {
	program := tea.NewProgram(
		tui.New(),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	_, err := program.Run()
	return err
}
