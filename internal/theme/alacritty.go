package theme

import (
    "path/filepath"
    "os"
    "strings"
    "gopkg.in/yaml.v3"
)

// Palette contains a subset of Alacritty's theme colors we care about.
type Palette struct {
    PrimaryBackground string
    PrimaryForeground string
    Normal            map[string]string
    Bright            map[string]string
}

// loadFromAlacritty attempts to read ~/.config/alacritty/alacritty.toml or .yml, follow
// import chains, and return merged colors. The importing file takes precedence, so
// the merge order is: imported files first, then the importer overlays them.
func loadFromAlacritty() (Palette, bool) {
    var pal Palette
    // Deprecated: we're using Omarchy as source of truth. Keep returning false.
    return pal, false
}

func parseAlacrittyWithImports(path string, visited map[string]struct{}) map[string]any {
    if _, seen := visited[path]; seen { return nil }
    visited[path] = struct{}{}
    data, err := os.ReadFile(path)
    if err != nil { return nil }
    var root map[string]any
    switch strings.ToLower(filepath.Ext(path)) {
    case ".yml", ".yaml":
        if err := yaml.Unmarshal(data, &root); err != nil { return nil }
    case ".toml":
        var tm map[string]any
        if err := decodeTOML(data, &tm); err != nil { return nil }
        root = tm
    default:
        return nil
    }
    dir := filepath.Dir(path)
    // Load imports first
    merged := map[string]any{}
    for _, imp := range extractImports(root) {
        // expand ~ and relative path
        imp = expandUser(imp)
        if !filepath.IsAbs(imp) {
            imp = filepath.Join(dir, imp)
        }
        // glob support
        paths := []string{imp}
        if hasGlob(imp) {
            gl, _ := filepath.Glob(imp)
            if len(gl) > 0 { paths = gl }
        }
        for _, p := range paths {
            if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
                child := parseAlacrittyWithImports(p, visited)
                merged = mergeMaps(merged, child)
            }
        }
    }
    // Then overlay current file
    merged = mergeMaps(merged, root)
    return merged
}

func extractImports(root map[string]any) []string {
    // Support both legacy top-level import and new [general] import
    v, ok := root["import"]
    if !ok {
        if gen, ok2 := root["general"].(map[string]any); ok2 {
            v, ok = gen["import"]
            if !ok { return nil }
        } else {
            return nil
        }
    }
    switch vv := v.(type) {
    case string:
        return []string{vv}
    case []any:
        out := make([]string, 0, len(vv))
        for _, it := range vv {
            if s, ok := it.(string); ok { out = append(out, s) }
        }
        return out
    default:
        return nil
    }
}

func extractPalette(root map[string]any) Palette {
    var pal Palette
    pal.Normal = map[string]string{}
    pal.Bright = map[string]string{}
    colors, _ := root["colors"].(map[string]any)
    if colors == nil { return pal }
    if prim, ok := colors["primary"].(map[string]any); ok {
        if bg, ok := prim["background"].(string); ok { pal.PrimaryBackground = normalizeHex(bg) }
        if fg, ok := prim["foreground"].(string); ok { pal.PrimaryForeground = normalizeHex(fg) }
    }
    if norm, ok := colors["normal"].(map[string]any); ok {
        for k, v := range norm {
            if s, ok := v.(string); ok { pal.Normal[k] = normalizeHex(s) }
        }
    }
    if br, ok := colors["bright"].(map[string]any); ok {
        for k, v := range br {
            if s, ok := v.(string); ok { pal.Bright[k] = normalizeHex(s) }
        }
    }
    return pal
}

func mergeMaps(a, b map[string]any) map[string]any {
    if a == nil { a = map[string]any{} }
    if b == nil { return a }
    for k, v := range b {
        if av, ok := a[k]; ok {
            ma, aok := av.(map[string]any)
            mb, bok := v.(map[string]any)
            if aok && bok {
                a[k] = mergeMaps(ma, mb)
                continue
            }
        }
        a[k] = v
    }
    return a
}

func hasGlob(s string) bool {
    return strings.ContainsAny(s, "*?[")
}

func expandUser(p string) string {
    if p == "" || p[0] != '~' { return p }
    home, err := os.UserHomeDir(); if err != nil { return p }
    if p == "~" { return home }
    return filepath.Join(home, p[2:])
}

// normalizeHex converts Alacritty-style hex like "0x1d2021" into "#1d2021".
func normalizeHex(s string) string {
    s = strings.TrimSpace(s)
    if s == "" { return s }
    if strings.HasPrefix(s, "0x") && len(s) == 8 {
        return "#" + s[2:]
    }
    if s[0] == '#' && len(s) == 7 { return s }
    // Some themes omit prefix; try to coerce 6-hex
    if len(s) == 6 { return "#" + s }
    return s
}

// decodeTOML unmarshals into a map using go-toml/v2 (loaded in another file).
func decodeTOML(data []byte, out *map[string]any) error {
    return decodeTOMLImpl(data, out)
}
