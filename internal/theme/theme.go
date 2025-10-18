package theme

import (
    "encoding/json"
    "os"
    "path/filepath"
    "strconv"
)

type Theme struct {
    Mode string // auto|dark|light|custom (resolved)
    Dark bool
    Colors Palette
}

// Detect attempts to resolve a dark/light theme from config and environment.
// MVP: honors explicit config; then tries pywal cache; otherwise dark.
func Detect(mode string) Theme {
    t := Theme{Mode: mode, Dark: true}
    // First: explicit overrides
    if mode == "dark" { t.Dark = true; return t }
    if mode == "light" { t.Dark = false; return t }
    // Auto: prefer Omarchy current theme
    if pal, ok := loadFromOmarchy(); ok {
        t.Colors = pal
        r, g, b, okc := hexToRGB(pal.PrimaryBackground)
        if okc {
            l := 0.2126*float64(r)/255 + 0.7152*float64(g)/255 + 0.0722*float64(b)/255
            t.Dark = l < 0.5
        }
        return t
    }
    // Next: pywal
    if isDarkFromPywal() {
        t.Dark = true
        return t
    }
    // Fallback
    t.Dark = true
    return t
}

// pywalColors is a subset of pywal colors.json
type pywalColors struct {
    Special struct {
        Background string `json:"background"`
    } `json:"special"`
}

func isDarkFromPywal() bool {
    home, err := os.UserHomeDir()
    if err != nil { return false }
    path := filepath.Join(home, ".cache", "wal", "colors.json")
    b, err := os.ReadFile(path)
    if err != nil { return false }
    var c pywalColors
    if err := json.Unmarshal(b, &c); err != nil { return false }
    if c.Special.Background == "" { return false }
    r, g, b2, ok := hexToRGB(c.Special.Background)
    if !ok { return false }
    // Perceived luminance
    l := 0.2126*float64(r)/255 + 0.7152*float64(g)/255 + 0.0722*float64(b2)/255
    return l < 0.5
}

func hexToRGB(s string) (int, int, int, bool) {
    // Accept formats like #rrggbb
    if len(s) != 7 || s[0] != '#' { return 0,0,0,false }
    r64, err := strconv.ParseUint(s[1:3], 16, 8)
    if err != nil { return 0,0,0,false }
    g64, err := strconv.ParseUint(s[3:5], 16, 8)
    if err != nil { return 0,0,0,false }
    b64, err := strconv.ParseUint(s[5:7], 16, 8)
    if err != nil { return 0,0,0,false }
    return int(r64), int(g64), int(b64), true
}
