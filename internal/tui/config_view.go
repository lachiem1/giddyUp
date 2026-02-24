package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lachiem1/giddyUp/internal/storage"
)

func renderConfigTitle() string {
	raw := []string{
		"█▀▀ █▀█ █▄ █ █▀▀ █ █▀▀",
		"█▄▄ █▄█ █ ▀█ █▀  █ █▄█",
		"▀▀▀ ▀▀▀ ▀  ▀ ▀   ▀ ▀▀▀",
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

func (m model) enterConfigView() (tea.Model, tea.Cmd) {
	m.selected = 0
	m.screen = screenConfig
	m.configErr = ""
	m.configFocus = 0
	m.configNextPayDigits = ""
	m.configDateDirty = false
	m.cmd.Blur()
	return m, m.loadConfigCmd()
}

func (m model) loadConfigCmd() tea.Cmd {
	return func() tea.Msg {
		if m.db == nil {
			return loadConfigMsg{err: fmt.Errorf("database is not initialized")}
		}
		repo := storage.NewAppConfigRepo(m.db)
		ctx := context.Background()

		nextDate, _, err := repo.Get(ctx, "pay_cycle.next_date")
		if err != nil {
			return loadConfigMsg{err: err}
		}
		freq, _, err := repo.Get(ctx, "pay_cycle.frequency")
		if err != nil {
			return loadConfigMsg{err: err}
		}
		return loadConfigMsg{
			nextPayDate: nextDate,
			frequency:   freq,
		}
	}
}

func (m model) saveConfigCmd(nextDate, frequency string) tea.Cmd {
	return func() tea.Msg {
		if m.db == nil {
			return saveConfigMsg{err: fmt.Errorf("database is not initialized"), silent: false}
		}
		repo := storage.NewAppConfigRepo(m.db)
		err := repo.UpsertMany(context.Background(), map[string]string{
			"pay_cycle.next_date": nextDate,
			"pay_cycle.frequency": frequency,
		})
		if err != nil {
			return saveConfigMsg{err: err, silent: false}
		}
		return saveConfigMsg{silent: false}
	}
}

func (m model) saveConfigDateCmd(nextDate string) tea.Cmd {
	return func() tea.Msg {
		if m.db == nil {
			return saveConfigMsg{err: fmt.Errorf("database is not initialized"), silent: true}
		}
		repo := storage.NewAppConfigRepo(m.db)
		err := repo.UpsertMany(context.Background(), map[string]string{
			"pay_cycle.next_date": nextDate,
		})
		if err != nil {
			return saveConfigMsg{err: err, silent: true}
		}
		return saveConfigMsg{silent: true}
	}
}

func configFrequencyOptions() []string {
	return []string{"weekly", "fortnightly", "monthly", "quarterly"}
}

func frequencyIndexFromValue(raw string) int {
	value := strings.ToLower(strings.TrimSpace(raw))
	opts := configFrequencyOptions()
	for i, v := range opts {
		if v == value {
			return i
		}
	}
	return 0
}

func dateToDigits(raw string) string {
	v := strings.TrimSpace(raw)
	if len(v) != 10 {
		return ""
	}
	if v[4] != '-' || v[7] != '-' {
		return ""
	}
	digits := strings.ReplaceAll(v, "-", "")
	if len(digits) != 8 {
		return ""
	}
	for _, ch := range digits {
		if ch < '0' || ch > '9' {
			return ""
		}
	}
	return digits
}

func validateAndFormatDateDigits(digits string, requireNotPast bool) (string, error) {
	if len(digits) != 8 {
		return "", fmt.Errorf("next pay date must be YYYY / MM / DD")
	}
	year, err := strconv.Atoi(digits[0:4])
	if err != nil || year < 2026 || year > 9999 {
		return "", fmt.Errorf("year must be 2026-9999")
	}
	month, err := strconv.Atoi(digits[4:6])
	if err != nil || month < 1 || month > 12 {
		return "", fmt.Errorf("month must be 01-12")
	}
	day, err := strconv.Atoi(digits[6:8])
	if err != nil || day < 1 || day > 31 {
		return "", fmt.Errorf("day must be 01-31")
	}
	date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.Local)
	if date.Year() != year || int(date.Month()) != month || date.Day() != day {
		return "", fmt.Errorf("date is not valid in the calendar")
	}
	if requireNotPast {
		now := time.Now().In(time.Local)
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
		if date.Before(today) {
			return "", fmt.Errorf("date cannot be in the past")
		}
	}
	return fmt.Sprintf("%04d-%02d-%02d", year, month, day), nil
}

func renderDateMask(digits string) string {
	d := make([]rune, 8)
	for i := range d {
		d[i] = '_'
	}
	for i, ch := range digits {
		if i >= len(d) {
			break
		}
		if ch >= '0' && ch <= '9' {
			d[i] = ch
		}
	}

	numStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB")).Bold(true)
	sepStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))
	part := func(start, end int) string { return numStyle.Render(string(d[start:end])) }
	return part(0, 4) + sepStyle.Render(" / ") + part(4, 6) + sepStyle.Render(" / ") + part(6, 8)
}

func (m model) renderConfigScreen(layoutWidth int) string {
	title := renderConfigTitle()
	title = lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, title)

	nextLabelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
	freqLabelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
	if m.configFocus == 0 {
		nextLabelStyle = nextLabelStyle.Bold(true)
	} else {
		freqLabelStyle = freqLabelStyle.Bold(true)
	}

	nextFieldBorder := lipgloss.Color("#FFFFFF")
	dateWarning := ""
	if m.configFocus == 0 {
		nextFieldBorder = lipgloss.Color("#FFD54A")
	}
	if len(m.configNextPayDigits) == 8 {
		if m.configDateDirty {
			if _, err := validateAndFormatDateDigits(m.configNextPayDigits, true); err != nil {
				nextFieldBorder = lipgloss.Color("#F15B5B")
				dateWarning = err.Error()
			} else {
				nextFieldBorder = lipgloss.Color("#5CCB76")
			}
		} else {
			nextFieldBorder = lipgloss.Color("#5CCB76")
		}
	}
	nextField := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(nextFieldBorder).
		Padding(0, 1).
		Render(renderDateMask(m.configNextPayDigits))

	opts := configFrequencyOptions()
	frequencyParts := make([]string, 0, len(opts))
	for i, opt := range opts {
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
		if i == m.configFrequencyIndex {
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true)
		}
		frequencyParts = append(frequencyParts, style.Render(opt))
	}
	frequencyLine := strings.Join(frequencyParts, "  ")
	freqBorder := lipgloss.Color("#FFFFFF")
	if m.configFocus == 1 {
		freqBorder = lipgloss.Color("#FFD54A")
	}
	frequencyField := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(freqBorder).
		Padding(0, 1).
		Render(frequencyLine)

	row1 := nextLabelStyle.Render("next pay date")
	row2 := nextField
	row3 := freqLabelStyle.Render("frequency")
	row4 := frequencyField
	row5 := lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).Render("tab/up/down switch field  left/right frequency")
	row6 := lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).Render("enter save all  esc back")

	contentWidth := max(
		lipgloss.Width(row1),
		max(
			lipgloss.Width(row2),
			max(
				lipgloss.Width(row3),
				max(lipgloss.Width(row4), max(lipgloss.Width(row5), lipgloss.Width(row6))),
			),
		),
	)
	center := func(s string) string {
		return lipgloss.PlaceHorizontal(contentWidth, lipgloss.Center, s)
	}

	content := []string{
		center(row1),
		center(row2),
		"",
		center(row3),
		center(row4),
		"",
		center(row5),
		center(row6),
	}
	warningText := strings.TrimSpace(m.configErr)
	if warningText == "" {
		warningText = strings.TrimSpace(dateWarning)
	}
	if warningText != "" {
		content = append(content, "", center(lipgloss.NewStyle().Foreground(lipgloss.Color("#F15B5B")).Render(warningText)))
	}

	panel := lipgloss.NewStyle().
		Padding(1, 2).
		Render(strings.Join(content, "\n"))
	panel = lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, panel)

	return strings.Join([]string{title, "", panel}, "\n")
}
