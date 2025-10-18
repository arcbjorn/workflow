package cache

import (
    "encoding/json"
    "errors"
    "os"
    "path/filepath"
    "time"
)

type RootCache struct {
    ScannedAt int64    `json:"scanned_at"`
    Repos     []string `json:"repos"`
}

type Data struct {
    Roots map[string]RootCache `json:"roots"`
}

func statePath() (string, error) {
    base := os.Getenv("XDG_STATE_HOME")
    if base == "" {
        home, err := os.UserHomeDir()
        if err != nil { return "", err }
        base = filepath.Join(home, ".local", "state")
    }
    dir := filepath.Join(base, "workflow")
    if err := os.MkdirAll(dir, 0o755); err != nil { return "", err }
    return filepath.Join(dir, "cache.json"), nil
}

func Load() (Data, error) {
    var d Data
    d.Roots = map[string]RootCache{}
    p, err := statePath()
    if err != nil { return d, err }
    b, err := os.ReadFile(p)
    if err != nil {
        if errors.Is(err, os.ErrNotExist) { return d, nil }
        return d, err
    }
    if err := json.Unmarshal(b, &d); err != nil { return d, err }
    if d.Roots == nil { d.Roots = map[string]RootCache{} }
    return d, nil
}

func Save(d Data) error {
    p, err := statePath()
    if err != nil { return err }
    b, err := json.MarshalIndent(d, "", "  ")
    if err != nil { return err }
    return os.WriteFile(p, b, 0o644)
}

// GetRepos returns cached repos for a root if within ttl, otherwise empty.
func GetRepos(d Data, root string, ttl time.Duration) []string {
    rc, ok := d.Roots[root]
    if !ok { return nil }
    if time.Since(time.Unix(rc.ScannedAt, 0)) > ttl { return nil }
    return append([]string(nil), rc.Repos...)
}

func PutRepos(d *Data, root string, repos []string) {
    if d.Roots == nil { d.Roots = map[string]RootCache{} }
    d.Roots[root] = RootCache{ScannedAt: time.Now().Unix(), Repos: append([]string(nil), repos...)}
}

