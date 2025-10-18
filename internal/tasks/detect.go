package tasks

import (
    "encoding/json"
    "os"
    "path/filepath"
    "sort"
    "strings"
)

type Task struct {
    Name string
    Cmd  string
    Src  string // e.g., node, go, rust, make, scripts
}

func Detect(path string) []Task {
    var out []Task
    // node scripts
    out = append(out, nodeTasks(path)...)
    // rust cargo
    out = append(out, rustTasks(path)...)
    // go module
    out = append(out, goTasks(path)...)
    // makefile
    out = append(out, makeTasks(path)...)
    // scripts directory
    out = append(out, scriptDirTasks(path)...)
    // de-dup by name+cmd
    out = dedup(out)
    // prioritize common tasks
    sort.SliceStable(out, func(i, j int) bool {
        wi := weight(out[i].Name)
        wj := weight(out[j].Name)
        if wi != wj { return wi < wj }
        if out[i].Src != out[j].Src { return out[i].Src < out[j].Src }
        return out[i].Name < out[j].Name
    })
    return out
}

func weight(name string) int {
    switch strings.ToLower(name) {
    case "dev", "start":
        return 0
    case "run":
        return 1
    case "test":
        return 2
    case "build":
        return 3
    case "lint", "fmt", "format":
        return 4
    default:
        return 10
    }
}

func dedup(in []Task) []Task {
    seen := map[string]struct{}{}
    var out []Task
    for _, t := range in {
        key := t.Name+"\x00"+t.Cmd
        if _, ok := seen[key]; ok { continue }
        seen[key] = struct{}{}
        out = append(out, t)
    }
    return out
}

// NodeJS detection
func nodeTasks(path string) []Task {
    pjPath := filepath.Join(path, "package.json")
    b, err := os.ReadFile(pjPath)
    if err != nil { return nil }
    var pkg struct {
        Scripts map[string]string `json:"scripts"`
        PackageManager string `json:"packageManager"`
    }
    if err := json.Unmarshal(b, &pkg); err != nil { return nil }
    if len(pkg.Scripts) == 0 { return nil }
    runner := pickNodeRunner(path, pkg.PackageManager)
    var out []Task
    for name := range pkg.Scripts {
        out = append(out, Task{Name: name, Cmd: runner+" run "+name, Src: "node"})
    }
    return out
}

func pickNodeRunner(path, pkgMgr string) string {
    // prefer pnpm
    if strings.HasPrefix(pkgMgr, "pnpm@") { return "pnpm" }
    if exists(filepath.Join(path, "pnpm-lock.yaml")) { return "pnpm" }
    if exists(filepath.Join(path, "yarn.lock")) { return "yarn" }
    if exists(filepath.Join(path, "package-lock.json")) { return "npm" }
    return "pnpm"
}

// Rust detection
func rustTasks(path string) []Task {
    if !exists(filepath.Join(path, "Cargo.toml")) { return nil }
    // Minimal set
    return []Task{
        {Name: "run", Cmd: "cargo run", Src: "rust"},
        {Name: "test", Cmd: "cargo test", Src: "rust"},
        {Name: "build", Cmd: "cargo build", Src: "rust"},
        {Name: "check", Cmd: "cargo check", Src: "rust"},
        {Name: "fmt", Cmd: "cargo fmt", Src: "rust"},
    }
}

// Go detection
func goTasks(path string) []Task {
    if !exists(filepath.Join(path, "go.mod")) { return nil }
    return []Task{
        {Name: "run", Cmd: "go run ./...", Src: "go"},
        {Name: "test", Cmd: "go test ./...", Src: "go"},
        {Name: "build", Cmd: "go build ./...", Src: "go"},
    }
}

// Makefile detection: common targets only
func makeTasks(path string) []Task {
    makefile := firstExisting(path, "Makefile", "makefile")
    if makefile == "" { return nil }
    data, err := os.ReadFile(makefile)
    if err != nil { return nil }
    // very simple target grep
    lines := strings.Split(string(data), "\n")
    wanted := []string{"dev","start","run","test","build","lint","fmt","format"}
    present := map[string]bool{}
    for _, ln := range lines {
        if strings.HasPrefix(ln, "#") { continue }
        if !strings.Contains(ln, ":") { continue }
        name := strings.TrimSpace(strings.SplitN(ln, ":", 2)[0])
        if name == "" || strings.Contains(name, "/") || strings.Contains(name, " ") { continue }
        for _, w := range wanted {
            if strings.EqualFold(name, w) { present[strings.ToLower(name)] = true }
        }
    }
    var out []Task
    for _, w := range wanted {
        if present[w] {
            out = append(out, Task{Name: w, Cmd: "make "+w, Src: "make"})
        }
    }
    return out
}

func scriptDirTasks(path string) []Task {
    dir := filepath.Join(path, "scripts")
    fi, err := os.Stat(dir)
    if err != nil || !fi.IsDir() { return nil }
    entries, err := os.ReadDir(dir)
    if err != nil { return nil }
    nameFor := map[string]string{
        "dev.sh":"dev","start.sh":"start","run.sh":"run","test.sh":"test","build.sh":"build","lint.sh":"lint","fmt.sh":"fmt","format.sh":"format",
    }
    var out []Task
    for _, e := range entries {
        if e.IsDir() { continue }
        if n, ok := nameFor[strings.ToLower(e.Name())]; ok {
            out = append(out, Task{Name: n, Cmd: "bash "+filepath.Join("scripts", e.Name()), Src: "scripts"})
        }
    }
    return out
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func firstExisting(base string, names ...string) string {
    for _, n := range names {
        p := filepath.Join(base, n)
        if exists(p) { return p }
    }
    return ""
}
