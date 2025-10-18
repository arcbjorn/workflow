package theme

import (
    "os"
    "path/filepath"
)

// loadFromOmarchy reads the current Omarchy theme colors by parsing the
// alacritty.toml within ~/.config/omarchy/current/theme.
func loadFromOmarchy() (Palette, bool) {
    var pal Palette
    base := os.Getenv("XDG_CONFIG_HOME")
    if base == "" {
        home, err := os.UserHomeDir(); if err != nil { return pal, false }
        base = filepath.Join(home, ".config")
    }
    p := filepath.Join(base, "omarchy", "current", "theme", "alacritty.toml")
    // Even though this is an Alacritty file, it is the Omarchy theme palette.
    // Reuse the existing TOML decoding and palette extraction.
    b, err := os.ReadFile(p)
    if err != nil { return pal, false }
    var root map[string]any
    if err := decodeTOML(b, &root); err != nil { return pal, false }
    pal = extractPalette(root)
    if pal.PrimaryBackground == "" && pal.PrimaryForeground == "" { return pal, false }
    return pal, true
}

