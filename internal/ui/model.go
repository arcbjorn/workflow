package ui

import (
    "fmt"
    "path/filepath"
    "os/exec"
    "sort"
    "strings"
    "time"

    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/bubbles/table"
    "github.com/charmbracelet/bubbles/textinput"
    "github.com/charmbracelet/bubbles/list"
    "github.com/charmbracelet/bubbles/spinner"
    "github.com/charmbracelet/bubbles/viewport"
    "github.com/charmbracelet/lipgloss"
    "github.com/charmbracelet/glamour"
    "workflow/internal/config"
    "workflow/internal/run"
    "workflow/internal/scanner"
    "workflow/internal/gitutil"
    "workflow/internal/theme"
    "workflow/internal/tasks"
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
    // Detail overlay
    showDetail bool
    detail     viewport.Model
    mdEnabled bool
    mdCache   map[string]string

    // Sorting
    sortKey string // last|ab|branch
    sortAsc bool

    // theme watch
    themeWatch bool
    // Grouping
    grouped  bool
    expanded map[string]bool // parent path -> expanded
    // Tasks overlay
    showTasks bool
    taskItems list.Model
    curTasks  []tasks.Task
    // Scan busy state
    scanning bool

    // Spinner for scanning
    spin spinner.Model

    // Last scan stats
    scanStart   time.Time
    lastScanDur time.Duration
    lastRepoCnt int
    lastRootsCnt int

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
        sortKey:     "last",
        sortAsc:     false,
        grouped:     false,
        expanded:    map[string]bool{},
        mdCache:     make(map[string]string),
    }
    // Spinner init
    sp := spinner.New()
    sp.Spinner = spinner.MiniDot
    sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(pickAccent(th.Colors, th.Dark)))
    m.spin = sp
    m.applyThemeToUI()
    m.setupAgentsList()
    m.updateTableHeader()
    // Detail viewport init
    vp := viewport.New(60, 12)
    vp.SetContent("")
    m.detail = vp
    return m
}

func (m Model) Init() tea.Cmd {
    // Start async scan and theme watch (Omarchy)
    return tea.Batch(
        func() tea.Msg { return startScanMsg{} },
        themeWatchStartCmd(),
        themeWatchWaitCmd(),
    )
}

type repoListMsg struct{ Entries []scanner.RepoEntry }
type detailMsg struct{ Text string }
type startScanMsg struct{}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case spinner.TickMsg:
        var cmd tea.Cmd
        m.spin, cmd = m.spin.Update(msg)
        return m, cmd
    case tea.WindowSizeMsg:
        m.width = msg.Width
        m.height = msg.Height
        m.updateTableHeader()
        m.updateTableHeight()
        // Resize agents list as overlay
        m.agents.SetSize(min(60, m.width-4), min(12, m.height-6))
        if m.showTasks {
            m.taskItems.SetSize(min(60, m.width-4), min(12, m.height-6))
        }
        m.detail.Width = min(m.width-6, 100)
        m.detail.Height = min(m.height-8, 20)
        return m, nil

    case repoListMsg:
        m.reposLoaded = true
        m.repos = orderRepos(msg.Entries, m.sortKey, m.sortAsc)
        m.refreshRows()
        m.scanning = false
        m.status = ""
        if !m.scanStart.IsZero() { m.lastScanDur = time.Since(m.scanStart) }
        m.lastRepoCnt = len(m.repos)
        m.lastRootsCnt = len(m.cfg.Roots)
        return m, nil
    case startScanMsg:
        if m.scanning { return m, nil }
        m.scanning = true
        m.status = "scanning…"
        m.scanStart = time.Now()
        return m, tea.Batch(scanCmd(m.cfg), m.spin.Tick)
    case themeTickMsg:
        // If theme changed, reapply palette
        if !themesEqual(m.th, msg.Theme) {
            m.th = msg.Theme
            m.applyThemeToUI()
            m.setupAgentsList()
            m.updateTableHeader()
        }
        // Wait for next change
        return m, themeWatchWaitCmd()
    case detailMsg:
        // Apply some minimal section styling on the first lines
        // First line is name, second path; then sections 'Recent commits' and 'README'
        if m.mdEnabled {
            // Render README (if present) in Markdown (theme-aware)
            content, ok := scanner.ReadmeContent(m.currentPath(), 1<<20)
            if ok {
                r, _ := glamour.NewTermRenderer(
                    glamour.WithAutoStyle(),
                    glamour.WithWordWrap(m.detail.Width),
                )
                if out, err := r.Render(content); err == nil {
                    m.detail.SetContent(out)
                    return m, nil
                }
            }
        }
        lines := strings.Split(msg.Text, "\n")
        if len(lines) > 0 {
            lines[0] = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(pickAccent(m.th.Colors, m.th.Dark))).Render(lines[0])
        }
        for i, ln := range lines {
            if ln == "Tasks (press r to run)" || ln == "Recent commits" || ln == "README" {
                lines[i] = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(pickAccent(m.th.Colors, m.th.Dark))).Render(ln)
            }
        }
        m.detail.SetContent(strings.Join(lines, "\n"))
        return m, nil

    case tea.KeyMsg:
        if m.showTasks {
            switch msg.String() {
            case "esc", "q":
                m.showTasks = false
                m.updateTableHeight()
                return m, nil
            case "enter":
                idx := m.taskItems.Index()
                if idx >= 0 && idx < len(m.curTasks) {
                    path := m.currentPath()
                    if path == "" { m.showTasks = false; return m, nil }
                    cmdStr := m.curTasks[idx].Cmd
                    if err := run.LaunchShellCmdNewWindow(path, cmdStr, m.cfg); err != nil {
                        m.status = "task: " + err.Error()
                    } else {
                        m.status = "task launched: " + m.curTasks[idx].Name
                    }
                    m.showTasks = false
                    return m, nil
                }
            }
            var cmd tea.Cmd
            m.taskItems, cmd = m.taskItems.Update(msg)
            return m, cmd
        }
        if m.showDetail {
            switch msg.String() {
            case "q", "esc", "enter":
                m.showDetail = false
                m.updateTableHeight()
                return m, nil
            case "M":
                // Toggle Markdown rendering for README while in details
                m.mdEnabled = !m.mdEnabled
                if len(m.visible) == 0 { return m, nil }
                idx := m.table.Cursor()
                if idx < 0 || idx >= len(m.visible) { return m, nil }
                ri := m.visible[idx]
                if ri < 0 || ri >= len(m.repos) { return m, nil }
                return m, loadDetailCmd(m.repos[ri])
            case "j":
                m.detail.ScrollDown(1)
                return m, nil
            case "k":
                m.detail.ScrollUp(1)
                return m, nil
            case "g":
                m.detail.GotoTop()
                return m, nil
            case "G":
                m.detail.GotoBottom()
                return m, nil
            }
            var cmd tea.Cmd
            m.detail, cmd = m.detail.Update(msg)
            return m, cmd
        }
        if m.showAgents {
            switch msg.String() {
            case "esc", "q":
                m.showAgents = false
                m.status = ""
                m.updateTableHeight()
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
        k := msg.String()
        // Map configured keys to internal defaults for switch handling
        k = m.mapKey(k)
        switch k {
        case "ctrl+c", "q":
            return m, tea.Quit
        case "?":
            m.showHelp = !m.showHelp
            return m, nil
        case "m":
            // Toggle grouped view
            m.grouped = !m.grouped
            if m.grouped {
                // Initialize all parents expanded by default
                for _, r := range m.repos {
                    if r.Monorepo {
                        if _, ok := m.expanded[r.Path]; !ok { m.expanded[r.Path] = true }
                    }
                }
            }
            m.refreshRows()
            m.status = map[bool]string{true:"grouped view", false:"flat view"}[m.grouped]
            return m, nil
        case "x":
            // Expand/collapse parent group
            if len(m.visible) == 0 { return m, nil }
            idx := m.table.Cursor()
            if idx < 0 || idx >= len(m.visible) { return m, nil }
            ri := m.visible[idx]
            if ri < 0 || ri >= len(m.repos) { return m, nil }
            r := m.repos[ri]
            var parent string
            if r.WorkspacePkg {
                parent = r.ParentPath
            } else if r.Monorepo {
                parent = r.Path
            }
            if parent != "" {
                m.expanded[parent] = !m.expanded[parent]
                m.refreshRows()
                return m, nil
            }
            return m, nil
        case "enter":
            // Open detail overlay and load content
            if len(m.visible) == 0 { return m, nil }
            idx := m.table.Cursor()
            if idx < 0 || idx >= len(m.visible) { return m, nil }
            ri := m.visible[idx]
            if ri < 0 || ri >= len(m.repos) { return m, nil }
            m.showDetail = true
            m.status = ""
            m.updateTableHeight()
            return m, loadDetailCmd(m.repos[ri])
        case "M":
            // Toggle markdown rendering of README in details
            m.mdEnabled = !m.mdEnabled
            if m.showDetail {
                // Synchronous re-render of detail content
                if len(m.visible) == 0 { return m, nil }
                idx := m.table.Cursor()
                if idx < 0 || idx >= len(m.visible) { return m, nil }
                ri := m.visible[idx]
                if ri < 0 || ri >= len(m.repos) { return m, nil }
                m.renderDetailNow(m.repos[ri])
                return m, nil
            }
            return m, nil
        case "/":
            m.filtering = true
            m.input.SetValue(m.filter)
            m.input.Focus()
            m.status = "type to filter; Enter apply; Esc cancel"
            return m, nil
        case "R":
            if m.scanning { m.status = "already scanning"; return m, nil }
            m.status = "refreshing…"
            return m, tea.Batch(scanCmd(m.cfg), m.spin.Tick)
        case "r":
            // Open tasks picker for current repo
            path := m.currentPath()
            if path == "" { return m, nil }
            ts := tasks.Detect(path)
            m.curTasks = ts
            items := make([]list.Item, 0, len(ts))
            for _, tsk := range ts {
                items = append(items, taskItem{Task: tsk})
            }
            if len(items) == 0 {
                m.status = "no tasks detected"
                return m, nil
            }
            li := list.New(items, list.NewDefaultDelegate(), 60, 12)
            li.Title = "Run task"
            s := li.Styles
            accHex := pickAccent(m.th.Colors, m.th.Dark)
            fg := pickFG(m.th.Colors, m.th.Dark)
            s.Title = lipgloss.NewStyle().Foreground(lipgloss.Color(accHex))
            s.NoItems = s.NoItems.Foreground(lipgloss.Color(fg))
            s.HelpStyle = s.HelpStyle.Foreground(lipgloss.Color(fg))
            li.Styles = s
            m.taskItems = li
            m.showTasks = true
            return m, nil
        case "s":
            // Cycle sort key: last -> ab -> branch -> last
            switch m.sortKey {
            case "last":
                m.sortKey = "ab"
            case "ab":
                m.sortKey = "branch"
            default:
                m.sortKey = "last"
            }
            m.repos = orderRepos(m.repos, m.sortKey, m.sortAsc)
            m.refreshRows()
            m.status = "sort: " + m.sortKey + map[bool]string{true:" asc", false:" desc"}[m.sortAsc]
            return m, nil
        case "S":
            m.sortAsc = !m.sortAsc
            m.repos = orderRepos(m.repos, m.sortKey, m.sortAsc)
            m.refreshRows()
            m.status = "sort: " + m.sortKey + map[bool]string{true:" asc", false:" desc"}[m.sortAsc]
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
        case "y":
            // copy path
            p := m.currentPath()
            if p == "" { m.status = "no selection"; return m, nil }
            if err := copyClipboard(p); err != nil { m.status = "copy: " + err.Error() } else { m.status = "copied path" }
            return m, nil
        case "u":
            // open remote URL
            p := m.currentPath()
            if p == "" { m.status = "no selection"; return m, nil }
            url, err := gitutil.RemoteURL(p)
            if err != nil || url == "" { m.status = "no remote"; return m, nil }
            _ = exec.Command("xdg-open", url).Start()
            m.status = "opened remote"
            return m, nil
        case "Y":
            // copy remote URL
            p := m.currentPath()
            if p == "" { m.status = "no selection"; return m, nil }
            url, err := gitutil.RemoteURL(p)
            if err != nil || url == "" { m.status = "no remote"; return m, nil }
            if err := copyClipboard(url); err != nil { m.status = "copy: " + err.Error() } else { m.status = "copied URL" }
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
    // indicators
    if m.grouped { title += "  [grouped]" }
    title += fmt.Sprintf("  [sort:%s%s]", m.sortKey, map[bool]string{true:"↑", false:"↓"}[m.sortAsc])
    if m.scanning {
        fmt.Fprintln(&b, titleStyle.Render(m.spin.View()+" "+title))
    } else {
        fmt.Fprintln(&b, titleStyle.Render(title))
    }
    // separator styled with a subtle theme foreground
    sep := strings.Repeat("─", max(10, m.width))
    fmt.Fprintln(&b, headerStyle.Faint(true).Render(sep))

    if !m.reposLoaded {
        fmt.Fprintln(&b, "loading repos…")
    } else if len(m.table.Rows()) == 0 {
        fmt.Fprintln(&b, "no projects found under configured roots")
    } else {
        fmt.Fprintln(&b, m.table.View())
    }

    overlayOpen := m.showAgents || m.showTasks || m.showDetail
    if !overlayOpen {
        if m.filtering {
            fmt.Fprintln(&b)
            fmt.Fprint(&b, "/ ")
            fmt.Fprintln(&b, m.input.View())
        }

        if m.status != "" {
            fmt.Fprintln(&b)
            if m.scanning {
                line := m.spin.View() + " " + m.status
                fmt.Fprintln(&b, statusStyle.Render(line))
            } else {
                fmt.Fprintln(&b, statusStyle.Render(m.status))
            }
        }

        // compact stats bar
        if m.reposLoaded {
            fmt.Fprintln(&b)
            dur := m.lastScanDur
            durStr := fmt.Sprintf("%dms", dur.Milliseconds())
            if dur.Seconds() >= 1 { durStr = fmt.Sprintf("%.1fs", dur.Seconds()) }
            stats := fmt.Sprintf("Roots: %d  Repos: %d  Scan: %s", m.lastRootsCnt, m.lastRepoCnt, durStr)
            fmt.Fprintln(&b, statusStyle.Render(stats))
        }
    }

    if m.showHelp && !overlayOpen {
        fmt.Fprintln(&b)
        fmt.Fprintln(&b, "j/k move  g/G home/end  / filter  R refresh  s/S sort  m group  x expand  ? help  q quit")
        fmt.Fprintln(&b, "Enter details  M toggle README MD  r tasks  e nvim  E GUI editor  o new shell  l lazygit  f fetch  a/A agents  y copy  u open URL  Y copy URL")
        // badges legend
        fmt.Fprintln(&b)
        legend := fmt.Sprintf("Badges: [%s dirty] [%s conflicts] [%s ahead] [%s behind] [%s detached] [%s parent] [%s pkg]",
            "*", "‼", "⇡", "⇣", "det", "mono", "pkg",
        )
        fmt.Fprintln(&b, legend)
    }
    if m.showAgents {
        fmt.Fprintln(&b)
        fmt.Fprintln(&b, m.agents.View())
    }
    if m.showTasks {
        fmt.Fprintln(&b)
        fmt.Fprintln(&b, m.taskItems.View())
    }
    if m.showDetail {
        fmt.Fprintln(&b)
        // Simple headline and instructions
        head := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(pickAccent(m.th.Colors, m.th.Dark))).Render("Details (Esc/Enter to close)")
        fmt.Fprintln(&b, head)
        fmt.Fprintln(&b, m.detail.View())
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

func orderRepos(in []scanner.RepoEntry, key string, asc bool) []scanner.RepoEntry {
    out := append([]scanner.RepoEntry(nil), in...)
    // stable sort: primary comparator by key, then fallback by name
    sort.SliceStable(out, func(i, j int) bool {
        // Always keep dirty first when sorting by recency/ab; for branch, do not force dirty grouping
        if key != "branch" && out[i].Dirty != out[j].Dirty {
            return out[i].Dirty && !out[j].Dirty
        }
        var less bool
        switch key {
        case "ab":
            ai := out[i].Ahead + out[i].Behind
            aj := out[j].Ahead + out[j].Behind
            if ai == aj {
                less = out[i].Name < out[j].Name
            } else {
                less = ai < aj
            }
        case "branch":
            if out[i].Branch == out[j].Branch {
                less = out[i].Name < out[j].Name
            } else {
                less = out[i].Branch < out[j].Branch
            }
        default: // "last"
            ai := ageScore(out[i].LastAge)
            aj := ageScore(out[j].LastAge)
            if ai == aj {
                less = out[i].Name < out[j].Name
            } else {
                less = ai < aj
            }
        }
        if asc {
            return less
        }
        return !less
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
    sel := m.table.Cursor()
    rowNo := 0
    if !m.grouped {
        for i, r := range m.repos {
            // per-repo hide override
            if m.isHidden(r.Path) { continue }
            if m.filter != "" {
                q := strings.ToLower(m.filter)
                if !strings.Contains(strings.ToLower(r.Name), q) && !strings.Contains(strings.ToLower(r.Branch), q) {
                    continue
                }
            }
            dirty := ""
            isSel := rowNo == sel
            if r.Dirty { dirty = "*" }
            ab := fmt.Sprintf("%d/%d", r.Ahead, r.Behind)
            state := bucket(r.LastAge)
            name := m.renderNameSelected(r, "", isSel)
            rows = append(rows, table.Row{name, state, r.Branch, dirty, ab, r.LastAge})
            m.visible = append(m.visible, i)
            rowNo++
        }
    } else {
        // Build child index and set for quick lookup
        childrenOf := map[string][]int{}
        childSet := map[int]bool{}
        for i, r := range m.repos {
            if r.WorkspacePkg && r.ParentPath != "" {
                childrenOf[r.ParentPath] = append(childrenOf[r.ParentPath], i)
                childSet[i] = true
            }
        }
        // helper to maybe render a row
        addRow := func(i int, r scanner.RepoEntry, indent string) {
            if m.filter != "" {
                q := strings.ToLower(m.filter)
                n := r.Name
                if r.WorkspacePkg && r.PackageName != "" { n = r.PackageName }
                if !strings.Contains(strings.ToLower(n), q) && !strings.Contains(strings.ToLower(r.Branch), q) {
                    return
                }
            }
            dirty := ""
            isSel := rowNo == sel
            if r.Dirty { dirty = "*" }
            ab := fmt.Sprintf("%d/%d", r.Ahead, r.Behind)
            state := bucket(r.LastAge)
            name := m.renderNameSelected(r, indent, isSel)
            rows = append(rows, table.Row{name, state, r.Branch, dirty, ab, r.LastAge})
            m.visible = append(m.visible, i)
            rowNo++
        }
        for i, r := range m.repos {
            if m.isHidden(r.Path) { continue }
            if r.WorkspacePkg { continue } // will be rendered under parent
            addRow(i, r, "")
            if r.Monorepo && m.expanded[r.Path] {
                // sort children by name
                ch := childrenOf[r.Path]
                sort.SliceStable(ch, func(a, b int) bool {
                    na := m.repos[ch[a]].Name
                    nb := m.repos[ch[b]].Name
                    if m.repos[ch[a]].PackageName != "" { na = m.repos[ch[a]].PackageName }
                    if m.repos[ch[b]].PackageName != "" { nb = m.repos[ch[b]].PackageName }
                    return na < nb
                })
                for _, ci := range ch {
                    if m.isHidden(m.repos[ci].Path) { continue }
                    addRow(ci, m.repos[ci], "  ")
                }
            }
        }
    }
    m.table.SetRows(rows)
    m.updateTableHeight()
}

func (m *Model) renderNameSelected(r scanner.RepoEntry, indent string, selected bool) string {
    // determine base display name
    base := r.Name
    if r.WorkspacePkg && r.PackageName != "" { base = r.PackageName }
    // override display name from config
    if name := m.overrideName(r.Path); name != "" { base = name }
    // colorized badges
    var parts []string
    if r.Dirty { parts = append(parts, "*") }
    if r.Conflicts > 0 { parts = append(parts, "‼") }
    if r.Ahead > 0 { parts = append(parts, "⇡") }
    if r.Behind > 0 { parts = append(parts, "⇣") }
    if strings.HasPrefix(strings.ToLower(r.Branch), "(detached)") { parts = append(parts, "det") }
    if r.Monorepo { parts = append(parts, "mono") }
    if r.WorkspacePkg { parts = append(parts, "pkg") }
    if len(parts) > 0 {
        return indent + fmt.Sprintf("%s [%s]", base, strings.Join(parts, ""))
    }
    return indent + base
}

func colorBadge(s string, th theme.Theme, key string) string {
    hex := ""
    if th.Dark {
        if v := th.Colors.Bright[key]; v != "" { hex = v }
    }
    if hex == "" {
        if v := th.Colors.Normal[key]; v != "" { hex = v } else if v := th.Colors.Bright[key]; v != "" { hex = v }
    }
    if hex == "" { hex = pickAccent(th.Colors, th.Dark) }
    st := lipgloss.NewStyle().Foreground(lipgloss.Color(hex))
    if !th.Dark { st = st.Faint(true) }
    return st.Render(s)
}

func (m *Model) isHidden(path string) bool {
    if len(m.cfg.Overrides) == 0 { return false }
    if ov, ok := m.cfg.Overrides[path]; ok {
        if ov.Hidden { return true }
    }
    for pat, ov := range m.cfg.Overrides {
        if strings.ContainsAny(pat, "*?[") {
            if ok, _ := filepath.Match(pat, path); ok {
                if ov.Hidden { return true }
            }
        }
    }
    return false
}

// mapKey maps a pressed key to the internal default binding if configured.
func (m *Model) mapKey(s string) string {
    if s == m.cfg.Keys.Group && s != "" { return "m" }
    if s == m.cfg.Keys.Expand && s != "" { return "x" }
    if s == m.cfg.Keys.Refresh && s != "" { return "R" }
    if s == m.cfg.Keys.Sort && s != "" { return "s" }
    if s == m.cfg.Keys.SortReverse && s != "" { return "S" }
    if s == m.cfg.Keys.Details && s != "" { return "enter" }
    if s == m.cfg.Keys.Tasks && s != "" { return "r" }
    return s
}

func (m *Model) overrideName(path string) string {
    if len(m.cfg.Overrides) == 0 { return "" }
    // exact path match first
    if ov, ok := m.cfg.Overrides[path]; ok {
        if ov.DisplayName != "" { return ov.DisplayName }
        return ""
    }
    // try glob patterns
    for pat, ov := range m.cfg.Overrides {
        if strings.ContainsAny(pat, "*?[") {
            if ok, _ := filepath.Match(pat, path); ok {
                if ov.DisplayName != "" { return ov.DisplayName }
            }
        }
    }
    return ""
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

type taskItem struct{ Task tasks.Task }

func (t taskItem) Title() string { return t.Task.Name }
func (t taskItem) Description() string { return t.Task.Cmd }
func (t taskItem) FilterValue() string { return t.Task.Name + " " + t.Task.Cmd }

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

// theme watching via polling
type themeTickMsg struct{ Theme theme.Theme }

func themeWatchCmd(cfg config.Config) tea.Cmd {
    return tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
        th := theme.Detect(cfg.Theme)
        return themeTickMsg{Theme: th}
    })
}

func themesEqual(a, b theme.Theme) bool {
    if a.Dark != b.Dark { return false }
    if a.Colors.PrimaryForeground != b.Colors.PrimaryForeground { return false }
    if a.Colors.PrimaryBackground != b.Colors.PrimaryBackground { return false }
    // compare a few key normals/bright to detect palette switch
    keys := []string{"black","red","green","yellow","blue","magenta","cyan","white"}
    for _, k := range keys {
        if a.Colors.Normal[k] != b.Colors.Normal[k] { return false }
        if a.Colors.Bright[k] != b.Colors.Bright[k] { return false }
    }
    return true
}

func (m *Model) applyThemeToUI() {
    applyThemePalette(m.th.Colors, m.th.Dark)
    st := table.DefaultStyles()
    fgHex := pickFG(m.th.Colors, m.th.Dark)
    accHex := pickAccent(m.th.Colors, m.th.Dark)
    st.Cell = st.Cell.Foreground(lipgloss.Color(fgHex))
    st.Header = st.Header.Foreground(lipgloss.Color(accHex)).Bold(true)
    selText := bestTextFor(accHex, fgHex, pickBG(m.th.Colors, m.th.Dark))
    st.Selected = st.Selected.Foreground(lipgloss.Color(selText)).Background(lipgloss.Color(accHex))
    m.table.SetStyles(st)
}

func (m *Model) setupAgentsList() {
    items := m.agentItems()
    d := list.NewDefaultDelegate()
    fgList := pickFG(m.th.Colors, m.th.Dark)
    accentHex := pickAccent(m.th.Colors, m.th.Dark)
    d.Styles.NormalTitle = d.Styles.NormalTitle.Foreground(lipgloss.Color(fgList))
    d.Styles.NormalDesc = d.Styles.NormalDesc.Foreground(lipgloss.Color(fgList))
    lstSelText := bestTextFor(accentHex, fgList, pickBG(m.th.Colors, m.th.Dark))
    d.Styles.SelectedTitle = d.Styles.SelectedTitle.
        Foreground(lipgloss.Color(lstSelText)).
        Background(lipgloss.Color(accentHex)).
        BorderForeground(lipgloss.Color(accentHex))
    d.Styles.SelectedDesc = d.Styles.SelectedDesc.
        Foreground(lipgloss.Color(lstSelText)).
        Background(lipgloss.Color(accentHex))
    lst := list.New(items, d, 40, 10)
    lst.Title = "Choose agent"
    lst.SetShowStatusBar(false)
    lst.SetFilteringEnabled(false)
    s := lst.Styles
    s.TitleBar = lipgloss.NewStyle()
    s.Title = lipgloss.NewStyle().Foreground(lipgloss.Color(accentHex))
    s.Spinner = s.Spinner.Foreground(lipgloss.Color(accentHex))
    s.FilterCursor = s.FilterCursor.Foreground(lipgloss.Color(accentHex))
    s.FilterPrompt = s.FilterPrompt.Foreground(lipgloss.Color(accentHex))
    s.DefaultFilterCharacterMatch = s.DefaultFilterCharacterMatch.Foreground(lipgloss.Color(accentHex))
    s.StatusBar = s.StatusBar.Foreground(lipgloss.Color(fgList))
    s.StatusEmpty = s.StatusEmpty.Foreground(lipgloss.Color(fgList))
    s.StatusBarActiveFilter = s.StatusBarActiveFilter.Foreground(lipgloss.Color(accentHex))
    s.StatusBarFilterCount = s.StatusBarFilterCount.Foreground(lipgloss.Color(fgList))
    s.NoItems = s.NoItems.Foreground(lipgloss.Color(fgList))
    s.HelpStyle = s.HelpStyle.Foreground(lipgloss.Color(fgList))
    s.ActivePaginationDot = s.ActivePaginationDot.Foreground(lipgloss.Color(accentHex))
    inactive := m.th.Colors.Normal["black"]
    if inactive == "" { inactive = fgList }
    s.InactivePaginationDot = s.InactivePaginationDot.Foreground(lipgloss.Color(inactive))
    s.ArabicPagination = s.ArabicPagination.Foreground(lipgloss.Color(inactive))
    s.DividerDot = s.DividerDot.Foreground(lipgloss.Color(inactive))
    lst.Styles = s
    m.agents = lst
}

func (m *Model) updateTableHeader() {
    // build columns with sort indicator and dynamic widths
    arrow := func(active bool) string {
        if !active { return "" }
        if m.sortAsc { return " ▲" }
        return " ▼"
    }
    // fixed widths for non-name columns
    wState := 7
    wBranch := 12
    wDelta := 2
    wAB := 7
    wLast := 6
    totalFixed := wState + wBranch + wDelta + wAB + wLast + 5 // padding between columns
    nameWidth := m.width - totalFixed
    if nameWidth < 16 { nameWidth = 16 }
    cols := []table.Column{
        {Title: "Name", Width: nameWidth},
        {Title: "State", Width: wState},
        {Title: "Branch" + arrow(m.sortKey=="branch"), Width: wBranch},
        {Title: "Δ", Width: wDelta},
        {Title: "A/B" + arrow(m.sortKey=="ab"), Width: wAB},
        {Title: "Last" + arrow(m.sortKey=="last"), Width: wLast},
    }
    m.table.SetColumns(cols)
    // ensure table width aligns with terminal width for consistent columns
    m.table.SetWidth(m.width)
}

func min(a, b int) int { if a<b { return a }; return b }

func loadDetailCmd(r scanner.RepoEntry) tea.Cmd {
    return func() tea.Msg {
        // Build detail content lazily (plain text; styling applied in View)
        return detailMsg{Text: buildDetailPlainText(r)}
    }
}

func buildDetailPlainText(r scanner.RepoEntry) string {
    var sb strings.Builder
    fmt.Fprintln(&sb, r.Name)
    fmt.Fprintf(&sb, "%s\n", r.Path)
    fmt.Fprintf(&sb, "Branch: %s\n", r.Branch)
    fmt.Fprintf(&sb, "Ahead/Behind: %d/%d\n", r.Ahead, r.Behind)
    fmt.Fprintf(&sb, "Dirty: %v  Conflicts: %d\n", r.Dirty, r.Conflicts)
    fmt.Fprintf(&sb, "Last: %s\n", r.LastAge)
    // Tasks preview
    fmt.Fprintln(&sb)
    fmt.Fprintln(&sb, "Tasks (press r to run)")
    ts := tasks.Detect(r.Path)
    if len(ts) == 0 {
        fmt.Fprintln(&sb, "(none)")
    } else {
        max := 8
        if len(ts) < max { max = len(ts) }
        for i := 0; i < max; i++ {
            fmt.Fprintf(&sb, "• %s — %s\n", ts[i].Name, ts[i].Cmd)
        }
        if len(ts) > max {
            fmt.Fprintf(&sb, "… and %d more\n", len(ts)-max)
        }
    }
    // Recent commits
    fmt.Fprintln(&sb)
    fmt.Fprintln(&sb, "Recent commits")
    commits := scanner.RecentCommits(r.Path, 8)
    if len(commits) == 0 {
        fmt.Fprintln(&sb, "No commits")
    } else {
        for _, c := range commits {
            fmt.Fprintln(&sb, c)
        }
    }
    // README snippet
    fmt.Fprintln(&sb)
    fmt.Fprintln(&sb, "README")
    readme := scanner.ReadmeSnippet(r.Path, 24)
    if len(readme) == 0 {
        fmt.Fprintln(&sb, "(none)")
    } else {
        for _, ln := range readme { fmt.Fprintln(&sb, ln) }
    }
    return sb.String()
}

func (m *Model) renderDetailNow(r scanner.RepoEntry) {
    if m.mdEnabled {
        key := fmt.Sprintf("%s#W%d#%t", r.Path, m.detail.Width, m.th.Dark)
        if cached, ok := m.mdCache[key]; ok && cached != "" {
            m.detail.SetContent(cached)
            m.detail.GotoTop()
            return
        }
        if content, ok := scanner.ReadmeContent(r.Path, 64*1024); ok { // limit for speed
            rmd, _ := glamour.NewTermRenderer(glamour.WithAutoStyle(), glamour.WithWordWrap(m.detail.Width))
            if out, err := rmd.Render(content); err == nil {
                m.mdCache[key] = out
                m.detail.SetContent(out)
                m.detail.GotoTop()
                return
            }
        }
    }
    m.detail.SetContent(buildDetailPlainText(r))
    m.detail.GotoTop()
}

// updateTableHeight computes table height so the overall view fits in the window.
func (m *Model) updateTableHeight() {
    overhead := 0
    // Title + separator
    overhead += 2
    // Filter input
    if m.filtering { overhead += 2 }
    // Status line
    if m.status != "" { overhead += 2 }
    // Stats bar
    if m.reposLoaded { overhead += 2 }
    // Help + legend
    if m.showHelp { overhead += 5 }

    contentH := m.height - overhead
    if contentH < 6 { contentH = 6 }

    if m.showAgents {
        // Agents overlay replaces content area; keep table height reasonable beneath
        ov := m.agents.Height()
        if ov <= 0 { ov = 10 }
        // reserve one blank line before overlay
        tableH := contentH - (1 + ov)
        if tableH < 3 { tableH = 3 }
        m.table.SetHeight(tableH)
        return
    }
    if m.showTasks {
        ov := m.taskItems.Height()
        if ov <= 0 { ov = 12 }
        tableH := contentH - (1 + ov)
        if tableH < 3 { tableH = 3 }
        m.table.SetHeight(tableH)
        return
    }
    if m.showDetail {
        // Allocate ~40% of the content to details with bounds
        detailH := int(float64(contentH) * 0.4)
        if detailH < 8 { detailH = 8 }
        if detailH > 24 { detailH = 24 }
        tableH := contentH - (1 + detailH) // one blank line spacing
        if tableH < 3 { tableH = 3 }
        m.detail.Height = detailH
        m.table.SetHeight(tableH)
        return
    }
    // Default: table fills content area
    m.table.SetHeight(contentH)
}

// computeLayout selects a responsive layout and sizes panes.
// computeLayout removed; single-column layout with responsive heights
