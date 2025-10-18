package scanner

import (
    "bufio"
    "encoding/json"
    "os"
    "os/exec"
    "path/filepath"
    "strconv"
    "strings"
    "sync"
    "time"

    "workflow/internal/config"
    "workflow/internal/cache"
    toml "github.com/pelletier/go-toml/v2"
    "gopkg.in/yaml.v3"
)

type RepoEntry struct {
    Name    string
    Path    string
    Branch  string
    Ahead   int
    Behind  int
    Dirty   bool
    Conflicts int
    LastAge string // e.g., 3d, 5h, 2mo
    Detached bool
    Monorepo bool       // parent has workspace members
    WorkspacePkg bool   // this entry is a workspace/package under a monorepo
    PackageName string  // optional package/crate name for workspace packages
    ParentPath  string  // parent repo root for workspace packages
}

// Scan finds git repos under roots (depth-limited) and collects status.
func Scan(cfg config.Config) ([]RepoEntry, error) {
    roots := make([]string, 0, len(cfg.Roots))
    for _, r := range cfg.Roots {
        roots = append(roots, config.ExpandUser(r))
    }
    found := map[string]struct{}{}
    var repos []string
    // Try cache first
    cd, _ := cache.Load()
    ttl := time.Duration(cfg.CacheTTLSeconds) * time.Second
    if ttl <= 0 { ttl = 120 * time.Second }
    for _, root := range roots {
        cached := cache.GetRepos(cd, root, ttl)
        if len(cached) > 0 {
            for _, p := range cached { if _, ok := found[p]; !ok { found[p]=struct{}{}; repos=append(repos,p) } }
            continue
        }
        rs, _ := findGitRepos(root, cfg.Depth)
        for _, p := range rs {
            if _, ok := found[p]; !ok {
                found[p] = struct{}{}
                repos = append(repos, p)
            }
        }
        // update cache for this root
        cache.PutRepos(&cd, root, rs)
    }
    _ = cache.Save(cd)
    // Concurrency limited scan for parent repos
    out := make([]RepoEntry, len(repos))
    var wg sync.WaitGroup
    sem := make(chan struct{}, max(8, 2*intConcurrency()))
    for i, p := range repos {
        i, p := i, p
        wg.Add(1)
        sem <- struct{}{}
        go func() {
            defer wg.Done()
            entry := collectRepo(p)
            entry.Name = filepath.Base(p)
            out[i] = entry
            <-sem
        }()
    }
    wg.Wait()

    // Discover monorepo workspace packages and append as separate rows
    // while marking parent as Monorepo
    parentIndex := map[string]int{}
    for i, e := range out { parentIndex[e.Path] = i }
    var children []RepoEntry
    for _, e := range out {
        ws := discoverWorkspaces(e.Path)
        if len(ws) > 0 {
            // mark parent
            if idx, ok := parentIndex[e.Path]; ok {
                out[idx].Monorepo = true
            }
            for _, c := range ws {
                child := collectRepo(c.Path)
                // prefer package name if available
                if c.PackageName != "" { child.Name = c.PackageName } else { child.Name = filepath.Base(c.Path) }
                child.WorkspacePkg = true
                child.ParentPath = e.Path
                child.PackageName = c.PackageName
                children = append(children, child)
            }
        }
    }
    // Combine and dedupe by path
    combined := append(out, children...)
    uniq := make([]RepoEntry, 0, len(combined))
    seen := map[string]struct{}{}
    for _, e := range combined {
        if _, ok := seen[e.Path]; ok { continue }
        seen[e.Path] = struct{}{}
        uniq = append(uniq, e)
    }
    return uniq, nil
}

func intConcurrency() int {
    n := 1
    if c := os.Getenv("GOMAXPROCS"); c != "" {
        if v, err := strconv.Atoi(c); err == nil && v > 0 { n = v }
    }
    if n < 1 { n = 1 }
    return n
}

func max(a, b int) int { if a>b { return a }; return b }

func findGitRepos(root string, depth int) ([]string, error) {
    var repos []string
    info, err := os.Stat(root)
    if err != nil || !info.IsDir() {
        return repos, err
    }
    baseDepth := depth
    // WalkDir with manual depth control
    filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
        if err != nil { return nil }
        // Depth calc
        rel, _ := filepath.Rel(root, path)
        if rel == "." { // root itself
            // Check if root is a repo too
            if isGitRepo(path) {
                repos = append(repos, path)
            }
            return nil
        }
        // Ignore heavy dirs by name
        if d.IsDir() {
            name := d.Name()
            if shouldIgnoreDir(name) {
                return filepath.SkipDir
            }
            // enforce depth
            depthNow := 1 + strings.Count(rel, string(os.PathSeparator))
            if depthNow > baseDepth {
                return filepath.SkipDir
            }
        }
        if d.IsDir() && isGitRepo(path) {
            repos = append(repos, path)
            return filepath.SkipDir
        }
        return nil
    })
    return repos, nil
}

func shouldIgnoreDir(name string) bool {
    switch name {
    case ".git", "node_modules", ".turbo", ".next", "dist", "build", "coverage", ".pnpm-store", "pnpm-store",
        "bin", "target", ".venv", "venv", "__pycache__", ".mypy_cache", ".pytest_cache", ".gradle", "obj":
        return true
    }
    return false
}

func isGitRepo(dir string) bool {
    // .git can be a dir or a file (submodule worktree)
    p := filepath.Join(dir, ".git")
    fi, err := os.Stat(p)
    if err != nil { return false }
    if fi.IsDir() { return true }
    if !fi.Mode().IsRegular() { return false }
    // .git file present → treat as repo
    return true
}

func collectRepo(path string) RepoEntry {
    st := RepoEntry{Path: path}
    // branch, ahead/behind, dirty/conflicts
    branch, ahead, behind, dirty, conflicts := parseStatus(path)
    st.Branch, st.Ahead, st.Behind, st.Dirty, st.Conflicts = branch, ahead, behind, dirty, conflicts
    if branch == "(detached)" { st.Detached = true }
    // last commit age
    st.LastAge = lastCommitAge(path)
    return st
}

func parseStatus(path string) (branch string, ahead, behind int, dirty bool, conflicts int) {
    cmd := exec.Command("git", "-C", path, "status", "--porcelain=v2", "-b")
    out, err := cmd.Output()
    if err != nil {
        return "", 0, 0, false, 0
    }
    s := bufio.NewScanner(strings.NewReader(string(out)))
    for s.Scan() {
        line := s.Text()
        if strings.HasPrefix(line, "# branch.head ") {
            branch = strings.TrimSpace(strings.TrimPrefix(line, "# branch.head "))
            // Detect detached state reported as "(detached)"
            if branch == "(detached)" {
                // keep label for caller to mark as detached
            }
        } else if strings.HasPrefix(line, "# branch.ab ") {
            // format: # branch.ab +A -B
            parts := strings.Fields(line)
            if len(parts) >= 4 {
                if strings.HasPrefix(parts[2], "+") { ahead, _ = strconv.Atoi(parts[2][1:]) }
                if strings.HasPrefix(parts[3], "-") { behind, _ = strconv.Atoi(parts[3][1:]) }
            }
        } else if len(line) > 0 {
            switch line[0] {
            case '1', '2':
                dirty = true
            case 'u':
                conflicts++
                dirty = true
            case '?':
                // untracked implies dirty
                dirty = true
            }
        }
    }
    return
}

func lastCommitAge(path string) string {
    cmd := exec.Command("git", "-C", path, "log", "-1", "--format=%ct")
    out, err := cmd.Output()
    if err != nil {
        return "—"
    }
    tsStr := strings.TrimSpace(string(out))
    sec, err := strconv.ParseInt(tsStr, 10, 64)
    if err != nil { return "—" }
    t := time.Unix(sec, 0)
    d := time.Since(t)
    if d < time.Hour {
        return "now"
    }
    if d < 24*time.Hour {
        return strconv.Itoa(int(d.Hours())) + "h"
    }
    if d < 30*24*time.Hour {
        return strconv.Itoa(int(d.Hours()/24)) + "d"
    }
    months := int(d.Hours() / (24*30))
    return strconv.Itoa(months) + "mo"
}

// Workspace discovery
type wsEntry struct {
    Path        string
    PackageName string
}

func discoverWorkspaces(repoRoot string) []wsEntry {
    var out []wsEntry
    // pnpm-workspace.yaml
    out = append(out, discoverPNPM(repoRoot)...)
    // Yarn/Node workspaces via package.json
    out = append(out, discoverNodeWorkspaces(repoRoot)...)
    // Cargo workspace
    out = append(out, discoverCargo(repoRoot)...)
    // Go nested modules
    out = append(out, discoverGoModules(repoRoot)...)
    // Git submodules
    out = append(out, discoverGitSubmodules(repoRoot)...)
    // de-dup
    seen := map[string]bool{}
    uniq := make([]wsEntry, 0, len(out))
    for _, e := range out {
        p, _ := filepath.Abs(e.Path)
        if !seen[p] {
            seen[p] = true
            uniq = append(uniq, e)
        }
    }
    return uniq
}

// Go modules beneath root (depth-limited)
func discoverGoModules(root string) []wsEntry {
    var out []wsEntry
    maxDepth := 2
    filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
        if err != nil { return nil }
        if !d.IsDir() { return nil }
        if path == root { return nil }
        rel, _ := filepath.Rel(root, path)
        depth := 1 + strings.Count(rel, string(os.PathSeparator))
        if depth > maxDepth { return filepath.SkipDir }
        if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
            out = append(out, wsEntry{Path: path, PackageName: goModuleName(path)})
            return filepath.SkipDir
        }
        return nil
    })
    return out
}

func goModuleName(dir string) string {
    b, err := os.ReadFile(filepath.Join(dir, "go.mod"))
    if err != nil { return "" }
    for _, ln := range strings.Split(string(b), "\n") {
        ln = strings.TrimSpace(ln)
        if strings.HasPrefix(ln, "module ") {
            mod := strings.TrimSpace(strings.TrimPrefix(ln, "module "))
            if i := strings.LastIndex(mod, "/"); i >= 0 { return mod[i+1:] }
            return mod
        }
    }
    return ""
}

// PNPM workspaces
func discoverPNPM(root string) []wsEntry {
    var out []wsEntry
    ypaths := []string{"pnpm-workspace.yaml", "pnpm-workspace.yml"}
    for _, n := range ypaths {
        p := filepath.Join(root, n)
        b, err := os.ReadFile(p)
        if err != nil { continue }
        // parse minimal YAML
        var node map[string]any
        if err := yaml.Unmarshal(b, &node); err != nil { continue }
        pkgs, _ := node["packages"].([]any)
        if pkgs == nil {
            // possibly string slice? yaml unmarshals into []interface{}
            // handle later
        }
        var globs []string
        switch v := node["packages"].(type) {
        case []any:
            for _, it := range v { if s, ok := it.(string); ok { globs = append(globs, s) } }
        case []string:
            globs = append(globs, v...)
        }
        for _, g := range globs {
            // skip negated globs for now
            if strings.HasPrefix(g, "!") { continue }
            // expand
            matches, _ := filepath.Glob(filepath.Join(root, g))
            for _, m := range matches {
                // only directories with package.json
                if fi, err := os.Stat(m); err == nil && fi.IsDir() {
                    if _, err := os.Stat(filepath.Join(m, "package.json")); err != nil { continue }
                    out = append(out, wsEntry{Path: m, PackageName: nodePackageName(m)})
                }
            }
        }
        break
    }
    return out
}

// Node workspaces via package.json workspaces field
func discoverNodeWorkspaces(root string) []wsEntry {
    pj := filepath.Join(root, "package.json")
    b, err := os.ReadFile(pj)
    if err != nil { return nil }
    var pkg struct {
        Workspaces any `json:"workspaces"`
    }
    if err := json.Unmarshal(b, &pkg); err != nil { return nil }
    var globs []string
    switch v := pkg.Workspaces.(type) {
    case []any:
        for _, it := range v { if s, ok := it.(string); ok { globs = append(globs, s) } }
    case map[string]any:
        if pkgs, ok := v["packages"].([]any); ok {
            for _, it := range pkgs { if s, ok := it.(string); ok { globs = append(globs, s) } }
        }
    }
    var out []wsEntry
    for _, g := range globs {
        if strings.HasPrefix(g, "!") { continue }
        matches, _ := filepath.Glob(filepath.Join(root, g))
        for _, m := range matches {
            if fi, err := os.Stat(m); err == nil && fi.IsDir() {
                if _, err := os.Stat(filepath.Join(m, "package.json")); err != nil { continue }
                out = append(out, wsEntry{Path: m, PackageName: nodePackageName(m)})
            }
        }
    }
    return out
}

func nodePackageName(dir string) string {
    b, err := os.ReadFile(filepath.Join(dir, "package.json"))
    if err != nil { return "" }
    var o struct{ Name string `json:"name"` }
    if err := json.Unmarshal(b, &o); err != nil { return "" }
    return o.Name
}

// Cargo workspace via Cargo.toml
func discoverCargo(root string) []wsEntry {
    var out []wsEntry
    b, err := os.ReadFile(filepath.Join(root, "Cargo.toml"))
    if err != nil { return nil }
    var tomlRoot map[string]any
    if err := toml.Unmarshal(b, &tomlRoot); err != nil { return nil }
    ws, _ := tomlRoot["workspace"].(map[string]any)
    if ws == nil { return nil }
    var members []string
    switch v := ws["members"].(type) {
    case []any:
        for _, it := range v { if s, ok := it.(string); ok { members = append(members, s) } }
    case []string:
        members = append(members, v...)
    }
    for _, g := range members {
        if strings.HasPrefix(g, "!") { continue }
        matches, _ := filepath.Glob(filepath.Join(root, g))
        for _, m := range matches {
            if fi, err := os.Stat(m); err == nil && fi.IsDir() {
                if _, err := os.Stat(filepath.Join(m, "Cargo.toml")); err != nil { continue }
                out = append(out, wsEntry{Path: m, PackageName: cargoPackageName(m)})
            }
        }
    }
    return out
}

func cargoPackageName(dir string) string {
    b, err := os.ReadFile(filepath.Join(dir, "Cargo.toml"))
    if err != nil { return "" }
    var t map[string]any
    if err := toml.Unmarshal(b, &t); err != nil { return "" }
    if pkg, ok := t["package"].(map[string]any); ok {
        if n, ok := pkg["name"].(string); ok { return n }
    }
    return ""
}

// Git submodules under root
func discoverGitSubmodules(root string) []wsEntry {
    b, err := os.ReadFile(filepath.Join(root, ".gitmodules"))
    if err != nil { return nil }
    lines := strings.Split(string(b), "\n")
    var out []wsEntry
    for _, ln := range lines {
        ln = strings.TrimSpace(ln)
        if strings.HasPrefix(ln, "path = ") {
            p := strings.TrimSpace(strings.TrimPrefix(ln, "path = "))
            full := filepath.Join(root, p)
            if fi, err := os.Stat(full); err == nil && fi.IsDir() {
                if isGitRepo(full) {
                    out = append(out, wsEntry{Path: full})
                }
            }
        }
    }
    return out
}
