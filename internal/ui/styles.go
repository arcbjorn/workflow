package ui

import (
    "github.com/charmbracelet/lipgloss"
    "workflow/internal/theme"
)

var (
    titleStyle    = lipgloss.NewStyle().Bold(true)
    headerStyle   = lipgloss.NewStyle().Bold(true)
    statusStyle   = lipgloss.NewStyle().Faint(true)
    selectedStyle = lipgloss.NewStyle().Bold(true)
)

// applyThemePalette applies the full palette. We avoid forcing a global
// background to respect the terminal background; selection uses the accent.
func applyThemePalette(p theme.Palette, dark bool) {
    fg := pickFG(p, dark)
    titleStyle = titleStyle.Foreground(lipgloss.Color(fg))
    headerStyle = headerStyle.Foreground(lipgloss.Color(fg))
    statusColor := p.Normal["cyan"]
    if statusColor == "" { statusColor = fg }
    statusStyle = statusStyle.Foreground(lipgloss.Color(statusColor))

    // Selected row uses accent as foreground or background; we set foreground
    // here and let components set background where appropriate.
    acc := pickAccent(p, dark)
    selectedStyle = selectedStyle.Foreground(lipgloss.Color(acc))
}

func pickAccent(p theme.Palette, dark bool) string {
    if v := p.Normal["blue"]; v != "" { return v }
    if v := p.Bright["blue"]; v != "" { return v }
    if v := p.Normal["cyan"]; v != "" { return v }
    if v := p.Bright["cyan"]; v != "" { return v }
    if p.PrimaryForeground != "" { return p.PrimaryForeground }
    if dark { return "#ffffff" }
    return "#000000"
}

func pickFG(p theme.Palette, dark bool) string {
    if p.PrimaryForeground != "" { return p.PrimaryForeground }
    if dark { return "#dddddd" }
    return "#1a1a1a"
}

func pickBG(p theme.Palette, dark bool) string {
    if p.PrimaryBackground != "" { return p.PrimaryBackground }
    if dark { return "#000000" }
    return "#ffffff"
}
