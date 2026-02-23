package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func renderAccountsTitle() string {
	raw := []string{
    "▄▀█ █▀▀ █▀▀ █▀█ █ █ █▄ █ ▀█▀ █▀",
    "█▀█ █▄▄ █▄▄ █▄█ █▄█ █ ▀█  █  ▄█",
    "▀ ▀ ▀▀▀ ▀▀▀ ▀▀▀ ▀▀▀ ▀  ▀  ▀  ▀▀",
	}
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#87CEEB")).
		Bold(true)
	rows := make([]string, 0, len(raw))
	for _, line := range raw {
		rows = append(rows, style.Render(line))
	}
	return strings.Join(rows, "\n")
}

func (m model) renderAccountsScreen(layoutWidth int) string {
	title := renderAccountsTitle()
	title = lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, title)
	return title
}

func renderStyledBlockTitle(raw []string, segments [][2]int) string {
	blue := lipgloss.NewStyle().Foreground(lipgloss.Color("#5FA8FF")).Bold(true)
	coral := lipgloss.NewStyle().Foreground(lipgloss.Color("#F47A60")).Bold(true)
	yellow := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD54A")).Bold(true)

	rows := make([]string, 0, len(raw))
	for _, line := range raw {
		runes := []rune(line)
		var out strings.Builder
		for idx, ch := range runes {
			if ch == ' ' {
				out.WriteRune(' ')
				continue
			}
			if isStrokeRune(ch) {
				out.WriteString(blue.Render(string(ch)))
				continue
			}
			seg := segmentForIndex(idx, segments)
			fill := coral
			if seg%2 == 1 {
				fill = yellow
			}
			out.WriteString(fill.Render(string(ch)))
		}
		rows = append(rows, out.String())
	}

	return strings.Join(rows, "\n")
}

func segmentRuns(line string) [][2]int {
	runes := []rune(line)
	segments := make([][2]int, 0, 8)
	start := -1
	for i, ch := range runes {
		if ch != ' ' {
			if start == -1 {
				start = i
			}
			continue
		}
		if start != -1 {
			segments = append(segments, [2]int{start, i - 1})
			start = -1
		}
	}
	if start != -1 {
		segments = append(segments, [2]int{start, len(runes) - 1})
	}
	if len(segments) == 0 {
		return [][2]int{{0, max(0, len(runes)-1)}}
	}
	return segments
}
