package ui

import (
    "strconv"

    "github.com/charmbracelet/lipgloss"
    "workflow/internal/theme"
)

var (
    titleStyle    = lipgloss.NewStyle().Bold(true)
    headerStyle   = lipgloss.NewStyle().Bold(true)
    statusStyle   = lipgloss.NewStyle()
    selectedStyle = lipgloss.NewStyle().Bold(true)
)

// applyThemePalette applies the full palette using only colors from the
// theme. We avoid forcing a global background to respect the terminal.
func applyThemePalette(p theme.Palette, dark bool) {
    fg := pickFG(p, dark)
    titleStyle = titleStyle.Foreground(lipgloss.Color(fg))
    // Separator/header uses a subdued palette color when available
    sep := p.Normal["black"]
    if sep == "" { sep = fg }
    headerStyle = headerStyle.Foreground(lipgloss.Color(sep))
    statusColor := p.Normal["cyan"]
    if statusColor == "" { statusColor = fg }
    statusStyle = statusStyle.Foreground(lipgloss.Color(statusColor))

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

// bestTextFor picks between the theme's primary foreground and primary
// background to maximize contrast against the given background color.
func bestTextFor(bgHex, primaryFGHex, primaryBGHex string) string {
    if bgHex == "" { return primaryFGHex }
    bl := luminanceHex(bgHex)
    fgl := luminanceHex(primaryFGHex)
    bgl := luminanceHex(primaryBGHex)
    if absf(bl-fgl) >= absf(bl-bgl) {
        if primaryFGHex != "" { return primaryFGHex }
        return primaryBGHex
    }
    if primaryBGHex != "" { return primaryBGHex }
    return primaryFGHex
}

func luminanceHex(h string) float64 {
    r, g, b, ok := hexToRGB(h)
    if !ok { return 0.5 }
    return 0.2126*float64(r)/255 + 0.7152*float64(g)/255 + 0.0722*float64(b)/255
}

func hexToRGB(s string) (int, int, int, bool) {
    if s == "" { return 0,0,0,false }
    if s[0] == '#' && len(s) == 7 {
        r64, err := strconv.ParseUint(s[1:3], 16, 8); if err != nil { return 0,0,0,false }
        g64, err := strconv.ParseUint(s[3:5], 16, 8); if err != nil { return 0,0,0,false }
        b64, err := strconv.ParseUint(s[5:7], 16, 8); if err != nil { return 0,0,0,false }
        return int(r64), int(g64), int(b64), true
    }
    return 0,0,0,false
}

func absf(f float64) float64 { if f < 0 { return -f }; return f }
