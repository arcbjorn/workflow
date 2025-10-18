package ui

import (
    "fmt"
    "os/exec"
    "sort"
    "strings"
    "time"

    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/bubbles/table"
    "github.com/charmbracelet/bubbles/textinput"
    "github.com/charmbracelet/bubbles/list"
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
    filter      string
    visible     []int // mapping of table row -> repos index

    // Config + theme
    cfg   config.Config
    th    theme.Theme

    // Status line
    status string

    // UI components
    table     table.Model
    input     textinput.Model
    filtering bool
    showAgents bool
    agents     list.Model
}

func NewModel(cfg config.Config, th theme.Theme) Model {
    columns := []table.Column{
        {Title: "Name", Width: 28},
        {Title: "State", Width: 7},
        {Title: "Branch", Width: 10},
        {Title: "Δ", Width: 2},
        {Title: "A/B", Width: 5},
        {Title: "Last", Width: 6},
    }
    t := table.New(table.WithColumns(columns), table.WithHeight(12))
    t.Focus()
    ti := textinput.New()
    ti.Placeholder = "type to filter; Enter apply, Esc cancel"
    ti.CharLimit = 64
    m := Model{
        showHelp:    true,
        reposLoaded: false,
        repos:       []scanner.RepoEntry{},
        filter:      "",
        visible:     []int{},
        cfg:         cfg,
        th:          th,
        status:      "",
        table:       t,
        input:       ti,
        filtering:   false,
        showAgents:  false,
    }
    // Agents list setup
    items := m.agentItems()
    lst := list.New(items, list.NewDefaultDelegate(), 40, 10)
    lst.Title = "Choose agent"
    lst.SetShowStatusBar(false)
    lst.SetFilteringEnabled(false)
    m.agents = lst
    return m
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
        h := m.height - 6
        if h < 5 { h = 5 }
        m.table.SetHeight(h)
        // Resize agents list as overlay
        m.agents.SetSize(min(60, m.width-4), min(12, m.height-6))
        return m, nil

    case repoListMsg:
        m.reposLoaded = true
        m.repos = orderRepos(msg.Entries)
        m.refreshRows()
        return m, nil

    case tea.KeyMsg:
        if m.showAgents {
            switch msg.String() {
            case "esc", "q":
                m.showAgents = false
                m.status = ""
                return m, nil
            case "enter":
                if it, ok := m.agents.SelectedItem().(agentItem); ok {
                    path := m.currentPath()
                    if path == "" { m.status = "no selection"; m.showAgents = false; return m, nil }
                    if err := run.LaunchAgentNewWindow(path, it.name, m.cfg); err != nil {
                        m.status = "agent: " + err.Error()
                    } else {
                        m.status = "agent launched: " + it.name
                    }
                    m.showAgents = false
                    return m, nil
                }
            }
            var cmd tea.Cmd
            m.agents, cmd = m.agents.Update(msg)
            return m, cmd
        }
        if m.filtering {
            switch msg.Type {
            case tea.KeyEsc:
                m.filtering = false
                m.input.Blur()
                m.status = "filter canceled"
                return m, nil
            case tea.KeyEnter:
                m.filter = m.input.Value()
                m.filtering = false
                m.input.Blur()
                m.status = "filter applied"
                m.refreshRows()
                return m, nil
            }
            var cmd tea.Cmd
            m.input, cmd = m.input.Update(msg)
            return m, cmd
        }
        switch msg.String() {
        case "ctrl+c", "q":
            return m, tea.Quit
        case "?":
            m.showHelp = !m.showHelp
            return m, nil
        case "/":
            m.filtering = true
            m.input.SetValue(m.filter)
            m.input.Focus()
            m.status = "type to filter; Enter apply; Esc cancel"
            return m, nil
        case "R":
            m.status = "refreshing…"
            return m, scanCmd(m.cfg)
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
            m.showAgents = true
            // Rebuild items in case config changed while running
            m.agents.SetItems(m.agentItems())
            m.status = ""
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
        default:
            var cmd tea.Cmd
            m.table, cmd = m.table.Update(msg)
            return m, cmd
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
    } else if len(m.table.Rows()) == 0 {
        fmt.Fprintln(&b, "no projects found under configured roots")
    } else {
        fmt.Fprintln(&b, m.table.View())
    }

    if m.filtering {
        fmt.Fprintln(&b)
        fmt.Fprint(&b, "/ ")
        fmt.Fprintln(&b, m.input.View())
    }

    if m.status != "" {
        fmt.Fprintln(&b)
        fmt.Fprintln(&b, m.status)
    }

    if m.showHelp {
        fmt.Fprintln(&b)
        fmt.Fprintln(&b, "j/k move  g/G home/end  / filter  R refresh  ? help  q quit")
        fmt.Fprintln(&b, "e nvim  E GUI editor  o new shell  l lazygit  f fetch  a/A agents")
    }
    if m.showAgents {
        fmt.Fprintln(&b)
        fmt.Fprintln(&b, m.agents.View())
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
    if len(m.visible) == 0 { return "" }
    idx := m.table.Cursor()
    if idx < 0 || idx >= len(m.visible) { return "" }
    ri := m.visible[idx]
    if ri < 0 || ri >= len(m.repos) { return "" }
    return m.repos[ri].Path
}

func orderRepos(in []scanner.RepoEntry) []scanner.RepoEntry {
    out := append([]scanner.RepoEntry(nil), in...)
    // Dirty first, then most recent activity
    sort.SliceStable(out, func(i, j int) bool {
        if out[i].Dirty != out[j].Dirty {
            return out[i].Dirty && !out[j].Dirty
        }
        return ageScore(out[i].LastAge) < ageScore(out[j].LastAge)
    })
    return out
}

func ageScore(a string) time.Duration {
    if a == "now" { return 0 }
    if strings.HasSuffix(a, "h") {
        n := 0
        fmt.Sscanf(a, "%dh", &n)
        return time.Duration(n) * time.Hour
    }
    if strings.HasSuffix(a, "d") {
        n := 0
        fmt.Sscanf(a, "%dd", &n)
        return time.Duration(n) * 24 * time.Hour
    }
    if strings.HasSuffix(a, "mo") {
        n := 0
        fmt.Sscanf(a, "%dmo", &n)
        return time.Duration(n) * 30 * 24 * time.Hour
    }
    return 999999 * time.Hour
}

func (m *Model) refreshRows() {
    rows := []table.Row{}
    m.visible = m.visible[:0]
    for i, r := range m.repos {
        if m.filter != "" {
            q := strings.ToLower(m.filter)
            if !strings.Contains(strings.ToLower(r.Name), q) && !strings.Contains(strings.ToLower(r.Branch), q) {
                continue
            }
        }
        dirty := ""
        if r.Dirty { dirty = "*" }
        ab := fmt.Sprintf("%d/%d", r.Ahead, r.Behind)
        state := bucket(r.LastAge)
        rows = append(rows, table.Row{r.Name, state, r.Branch, dirty, ab, r.LastAge})
        m.visible = append(m.visible, i)
    }
    m.table.SetRows(rows)
}

type agentItem struct{
    name string
    cmd  string
    def  bool
}

func (a agentItem) Title() string {
    if a.def { return a.name + " (default)" }
    return a.name
}
func (a agentItem) Description() string { return a.cmd }
func (a agentItem) FilterValue() string { return a.name + " " + a.cmd }

func (m *Model) agentItems() []list.Item {
    items := []list.Item{}
    // stable order: default first, then alphabetical others
    keys := make([]string, 0, len(m.cfg.Agents.Map))
    for k := range m.cfg.Agents.Map { keys = append(keys, k) }
    sort.Strings(keys)
    def := m.cfg.Agents.Default
    if def != "" {
        cmd := m.cfg.Agents.Map[def]
        if cmd == "" { cmd = def }
        items = append(items, agentItem{name: def, cmd: cmd, def: true})
        // remove def from keys if present
        out := keys[:0]
        for _, k := range keys { if k != def { out = append(out, k) } }
        keys = out
    }
    for _, k := range keys {
        cmd := m.cfg.Agents.Map[k]
        if cmd == "" { cmd = k }
        items = append(items, agentItem{name: k, cmd: cmd})
    }
    return items
}

func scanCmd(cfg config.Config) tea.Cmd {
    return func() tea.Msg {
        entries, _ := scanner.Scan(cfg)
        return repoListMsg{Entries: entries}
    }
}
