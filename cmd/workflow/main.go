package main

import (
    "log"

    tea "github.com/charmbracelet/bubbletea"
    "workflow/internal/config"
    "workflow/internal/theme"
    "workflow/internal/ui"
)

func main() {
    cfg, err := config.Load()
    if err != nil {
        log.Fatal(err)
    }
    th := theme.Detect(cfg.Theme)
    p := tea.NewProgram(ui.NewModel(cfg, th), tea.WithAltScreen())
    if err := p.Start(); err != nil {
        log.Fatal(err)
    }
}
