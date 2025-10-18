package scanner

import (
    "bufio"
    "os"
    "os/exec"
    "path/filepath"
    "strconv"
    "strings"
    "sync"
    "time"

    "workflow/internal/config"
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
}

// Scan finds git repos under roots (depth-limited) and collects status.
func Scan(cfg config.Config) ([]RepoEntry, error) {
    roots := make([]string, 0, len(cfg.Roots))
    for _, r := range cfg.Roots {
        roots = append(roots, config.ExpandUser(r))
    }
    found := map[string]struct{}{}
    var repos []string
    for _, root := range roots {
        rs, _ := findGitRepos(root, cfg.Depth)
        for _, p := range rs {
            if _, ok := found[p]; !ok {
                found[p] = struct{}{}
                repos = append(repos, p)
            }
        }
    }
    // Concurrency limited scan
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
    return out, nil
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
