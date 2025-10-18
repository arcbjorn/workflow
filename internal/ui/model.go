package ui

import (
    "fmt"
    "os/exec"
    "strings"

    tea "github.com/charmbracelet/bubbletea"
    "workflow/internal/config"
    "workflow/internal/run"
    "workflow/internal/scanner"
    "workflow/internal/theme"
)

type Model struct {
    width  int
    height int
    showHelp bool

    // Data
    reposLoaded bool
    repos       []scanner.RepoEntry
    selected    int
    filter      string

    // Config + theme
    cfg   config.Config
    th    theme.Theme

    // Status line
    status string
}

func NewModel(cfg config.Config, th theme.Theme) Model {
    return Model{
        showHelp:   true,
        reposLoaded: false,
        repos:       []scanner.RepoEntry{},
        selected:    0,
        filter:      "",
        cfg:         cfg,
        th:          th,
        status:      "",
    }
}

func (m Model) Init() tea.Cmd {
    // Start async scan
    return func() tea.Msg {
        entries, _ := scanner.Scan(m.cfg)
        return repoListMsg{Entries: entries}
    }
}

type repoListMsg struct{ Entries []scanner.RepoEntry }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.WindowSizeMsg:
        m.width = msg.Width
        m.height = msg.Height
        return m, nil

    case repoListMsg:
        m.reposLoaded = true
        m.repos = msg.Entries
        if m.selected >= len(m.repos) { m.selected = 0 }
        return m, nil

    case tea.KeyMsg:
        switch msg.String() {
        case "ctrl+c", "q":
            return m, tea.Quit
        case "?":
            m.showHelp = !m.showHelp
            return m, nil
        case "j", "down":
            if len(m.repos) > 0 && m.selected < len(m.repos)-1 {
                m.selected++
            }
            return m, nil
        case "k", "up":
            if m.selected > 0 {
                m.selected--
            }
            return m, nil
        case "/":
            // Filter stub: toggle help to indicate placeholder
            m.showHelp = true
            return m, nil
        case "R":
            m.status = "refresh queued"
            return m, nil
        case "e":
            path := m.currentPath()
            if path == "" { m.status = "no selection"; return m, nil }
            ed := m.cfg.Editor.Default
            if ed == "" { ed = "nvim" }
            if err := run.LaunchShellCmdNewWindow(path, ed+" "+path, m.cfg); err != nil {
                m.status = "editor: " + err.Error()
            } else {
                m.status = "opened editor"
            }
            return m, nil
        case "E":
            path := m.currentPath()
            if path == "" { m.status = "no selection"; return m, nil }
            if err := run.OpenGUIEditor(path, m.cfg); err != nil {
                m.status = "GUI editor: " + err.Error()
            } else {
                m.status = "opened GUI editor"
            }
            return m, nil
        case "o":
            path := m.currentPath()
            if path == "" { m.status = "no selection"; return m, nil }
            if err := run.OpenTerminalNewWindow(path, m.cfg); err != nil {
                m.status = "terminal: " + err.Error()
            } else {
                m.status = "opened terminal"
            }
            return m, nil
        case "l":
            path := m.currentPath()
            if path == "" { m.status = "no selection"; return m, nil }
            if err := run.LaunchShellCmdNewWindow(path, "lazygit", m.cfg); err != nil {
                m.status = "lazygit: " + err.Error()
            } else {
                m.status = "lazygit launched"
            }
            return m, nil
        case "f":
            path := m.currentPath()
            if path == "" { m.status = "no selection"; return m, nil }
            m.status = "fetching…"
            go func() { _ = exec.Command("git", "-C", path, "fetch", "--all", "--prune").Run() }()
            return m, nil
        case "a":
            m.status = "agents picker — TODO"
            return m, nil
        case "A":
            path := m.currentPath()
            if path == "" { m.status = "no selection"; return m, nil }
            agent := m.cfg.Agents.Default
            if agent == "" { agent = "claude" }
            if err := run.LaunchAgentNewWindow(path, agent, m.cfg); err != nil {
                m.status = "agent: " + err.Error()
            } else {
                m.status = "agent launched"
            }
            return m, nil
        }
    }
    return m, nil
}

func (m Model) View() string {
    var b strings.Builder

    title := "workflow — projects"
    if m.filter != "" {
        title += fmt.Sprintf("  [/%s]", m.filter)
    }
    fmt.Fprintln(&b, title)
    fmt.Fprintln(&b, strings.Repeat("─", max(10, m.width)))

    if !m.reposLoaded {
        fmt.Fprintln(&b, "loading repos…")
    } else if len(m.repos) == 0 {
        fmt.Fprintln(&b, "no projects found under configured roots")
    } else {
        // Header
        fmt.Fprintln(&b, "Name                         State  Branch   Δ  A/B  Last")
        fmt.Fprintln(&b, strings.Repeat("-", max(10, m.width)))
        for i, r := range m.repos {
            cursor := " "
            if i == m.selected {
                cursor = ">"
            }
            dirty := ""
            if r.Dirty { dirty = "*" }
            ab := fmt.Sprintf("%d/%d", r.Ahead, r.Behind)
            // State derived from last age bucket (simple heuristic)
            state := bucket(r.LastAge)
            name := r.Name
            fmt.Fprintf(&b, "%s %-28s %-6s %-7s %-2s %-3s %-4s\n",
                cursor, name, state, r.Branch, dirty, ab, r.LastAge)
        }
    }

    if m.status != "" {
        fmt.Fprintln(&b)
        fmt.Fprintln(&b, m.status)
    }

    if m.showHelp {
        fmt.Fprintln(&b)
        fmt.Fprintln(&b, "j/k move  Enter details  / filter  R refresh  ? help  q quit")
        fmt.Fprintln(&b, "e nvim  E GUI editor  o new shell  l lazygit  f fetch  a/A agents")
    }
    return b.String()
}

func max(a, b int) int {
    if a > b {
        return a
    }
    return b
}

func bucket(age string) string {
    // age is formatted already; use rough mapping
    if age == "now" { return "active" }
    if strings.HasSuffix(age, "h") { return "active" }
    if strings.HasSuffix(age, "d") {
        // parse days
        d := 0
        fmt.Sscanf(age, "%dd", &d)
        switch {
        case d <= 3:
            return "active"
        case d <= 14:
            return "warm"
        case d <= 45:
            return "stale"
        default:
            return "dormant"
        }
    }
    return "stale"
}

func (m Model) currentPath() string {
    if len(m.repos) == 0 { return "" }
    if m.selected < 0 || m.selected >= len(m.repos) { return "" }
    return m.repos[m.selected].Path
}
