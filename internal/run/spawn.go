package run

import (
    "fmt"
    "os/exec"
    "strings"

    "workflow/internal/agents"
    "workflow/internal/config"
)

func OpenTerminalNewWindow(cwd string, cfg config.Config) error {
    term := cfg.Terminal.Prefer
    switch term {
    case "alacritty", "":
        if !agents.HasBinary("alacritty") {
            return fmt.Errorf("alacritty not found")
        }
        cmd := exec.Command("alacritty", agents.BuildAlacrittyArgs(cwd, cfg.Terminal.InPlaceShell+" -lc \"$SHELL\"")...)
        return cmd.Start()
    default:
        return fmt.Errorf("unsupported terminal: %s", term)
    }
}

func LaunchAgentNewWindow(cwd, agent string, cfg config.Config) error {
    if !agents.HasBinary("alacritty") {
        return fmt.Errorf("alacritty not found")
    }
    sh := agents.BuildAgentCommand(agent, cwd, cfg)
    // Ensure quotes are escaped for bash -lc
    sh = strings.ReplaceAll(sh, "\"", "\\\"")
    args := agents.BuildAlacrittyArgs(cwd, sh)
    cmd := exec.Command("alacritty", args...)
    return cmd.Start()
}

func LaunchShellCmdNewWindow(cwd, shellCmd string, cfg config.Config) error {
    if !agents.HasBinary("alacritty") {
        return fmt.Errorf("alacritty not found")
    }
    // Escape quotes for bash -lc
    shellCmd = strings.ReplaceAll(shellCmd, "\"", "\\\"")
    args := agents.BuildAlacrittyArgs(cwd, shellCmd)
    cmd := exec.Command("alacritty", args...)
    return cmd.Start()
}
