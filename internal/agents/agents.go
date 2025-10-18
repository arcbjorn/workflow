package agents

import (
    "os/exec"
    "strings"

    "workflow/internal/config"
)

// BuildAgentCommand returns a shell command string to launch the agent.
func BuildAgentCommand(agent string, cwd string, cfg config.Config) string {
    cmd := cfg.Agents.Map[agent]
    if cmd == "" {
        cmd = agent
    }
    parts := append([]string{}, cfg.Agents.Prelude...)
    parts = append(parts, cmd)
    chain := strings.Join(parts, "; ")
    tpl := cfg.Agents.CmdTemplate
    if tpl == "" {
        tpl = "cd {cwd} && {cmd}"
    }
    return strings.NewReplacer("{cwd}", cwd, "{cmd}", chain).Replace(tpl)
}

// BuildAlacrittyArgs builds an argument slice to run a shell command in a new
// Alacritty window with working directory.
func BuildAlacrittyArgs(cwd, shellCmd string) []string {
    // alacritty --working-directory <cwd> -e bash -lc '<cmd>'
    return []string{"--working-directory", cwd, "-e", "bash", "-lc", shellCmd}
}

func HasBinary(name string) bool {
    _, err := exec.LookPath(name)
    return err == nil
}
