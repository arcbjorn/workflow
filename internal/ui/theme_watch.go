package ui

import (
    "os"
    "path/filepath"
    "sync"

    "github.com/fsnotify/fsnotify"
    tea "github.com/charmbracelet/bubbletea"
    "workflow/internal/theme"
)

var (
    watchOnce sync.Once
    watchInitErr error
    watch *fsnotify.Watcher
    watchCh = make(chan struct{}, 8)
)

func omarchyPaths() []string {
    base := os.Getenv("XDG_CONFIG_HOME")
    if base == "" {
        if home, err := os.UserHomeDir(); err == nil {
            base = filepath.Join(home, ".config")
        }
    }
    if base == "" { return nil }
    current := filepath.Join(base, "omarchy", "current")
    themeDir := filepath.Join(current, "theme")
    themeFile := filepath.Join(themeDir, "alacritty.toml")
    return []string{current, themeDir, themeFile}
}

func startThemeWatcher() error {
    var err error
    watch, err = fsnotify.NewWatcher()
    if err != nil { return err }
    for _, p := range omarchyPaths() {
        _ = watch.Add(p)
    }
    go func() {
        for {
            select {
            case <-watch.Events:
                select { case watchCh <- struct{}{}: default: }
            case <-watch.Errors:
                // ignore; next event will retrigger
            }
        }
    }()
    return nil
}

func themeWatchStartCmd() tea.Cmd {
    return func() tea.Msg {
        watchOnce.Do(func(){ watchInitErr = startThemeWatcher() })
        // send an immediate event if started ok to apply any theme now
        if watchInitErr == nil {
            select { case watchCh <- struct{}{}: default: }
        }
        // Report initial theme regardless
        th := theme.Detect("auto")
        return themeTickMsg{Theme: th}
    }
}

func themeWatchWaitCmd() tea.Cmd {
    return func() tea.Msg {
        <-watchCh
        th := theme.Detect("auto")
        return themeTickMsg{Theme: th}
    }
}

