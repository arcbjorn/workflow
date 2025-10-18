package ui

import (
    "github.com/charmbracelet/lipgloss"
)

var (
    titleStyle    = lipgloss.NewStyle().Bold(true)
    headerStyle   = lipgloss.NewStyle().Bold(true)
    statusStyle   = lipgloss.NewStyle().Faint(true)
    selectedStyle = lipgloss.NewStyle().Bold(true)
)

func applyThemeColors(fg, bg, accent string) {
    if fg != "" { titleStyle = titleStyle.Foreground(lipgloss.Color(fg)); headerStyle = headerStyle.Foreground(lipgloss.Color(fg)); statusStyle = statusStyle.Foreground(lipgloss.Color(fg)) }
    if bg != "" { titleStyle = titleStyle.Background(lipgloss.Color(bg)); headerStyle = headerStyle.Background(lipgloss.Color(bg)) }
    if accent != "" { selectedStyle = selectedStyle.Foreground(lipgloss.Color(accent)) }
}
