package scanner

import (
    "bufio"
    "os"
    "os/exec"
    "path/filepath"
    "sort"
    "strconv"
    "strings"
)

// RecentCommits returns last n commit summaries formatted as "%h %s • %cr".
func RecentCommits(path string, n int) []string {
    if n <= 0 { return nil }
    cmd := exec.Command("git", "-C", path, "--no-pager", "log", "--pretty=format:%h %s • %cr", "-n", strconv.Itoa(n))
    out, err := cmd.Output()
    if err != nil { return nil }
    lines := splitNonEmpty(string(out))
    return lines
}

// ReadmeSnippet returns up to maxLines of the first README file we can find.
func ReadmeSnippet(path string, maxLines int) []string {
    if maxLines <= 0 { return nil }
    name := findReadme(path)
    if name == "" { return nil }
    f, err := os.Open(filepath.Join(path, name))
    if err != nil { return nil }
    defer f.Close()
    var out []string
    s := bufio.NewScanner(f)
    for s.Scan() {
        out = append(out, s.Text())
        if len(out) >= maxLines { break }
    }
    return out
}

func findReadme(path string) string {
    entries, err := os.ReadDir(path)
    if err != nil { return "" }
    candidates := []string{}
    for _, e := range entries {
        if e.IsDir() { continue }
        n := strings.ToLower(e.Name())
        if strings.HasPrefix(n, "readme") {
            candidates = append(candidates, e.Name())
        }
    }
    if len(candidates) == 0 { return "" }
    sort.Strings(candidates)
    return candidates[0]
}

func splitNonEmpty(s string) []string {
    var out []string
    for _, ln := range strings.Split(s, "\n") {
        ln = strings.TrimRight(ln, "\r")
        if ln != "" { out = append(out, ln) }
    }
    return out
}
