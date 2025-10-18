package config

import (
    "errors"
    "io/fs"
    "os"
    "path/filepath"

    "gopkg.in/yaml.v3"
)

type Editors struct {
    Default      string   `yaml:"default"`
    GUIFallbacks []string `yaml:"gui_fallbacks"`
}

type Terminal struct {
    Prefer      string `yaml:"prefer"`
    InPlaceShell string `yaml:"in_place_shell"`
}

type Agents struct {
    Default     string            `yaml:"default"`
    Map         map[string]string `yaml:"map"`
    Prelude     []string          `yaml:"prelude"`
    CmdTemplate string            `yaml:"cmd_template"`
}

type Config struct {
    Roots  []string `yaml:"roots"`
    Depth  int      `yaml:"depth"`
    Ignore string   `yaml:"ignore"`

    Editor   Editors  `yaml:"editor"`
    Terminal Terminal `yaml:"terminal"`
    Agents   Agents   `yaml:"agents"`

    Theme string `yaml:"theme"`
}

func Default() Config {
    return Config{
        Roots:  []string{"~/projects"},
        Depth:  2,
        Ignore: "auto",
        Editor: Editors{
            Default:      "nvim",
            GUIFallbacks: []string{"cursor", "code"},
        },
        Terminal: Terminal{
            Prefer:      "alacritty",
            InPlaceShell: os.Getenv("SHELL"),
        },
        Agents: Agents{
            Default:     "claude",
            Map:         map[string]string{"claude": "claude", "gemini": "gemini", "codex": "openai chat", "opencode": "opencode"},
            Prelude:     []string{},
            CmdTemplate: "cd {cwd} && {cmd}",
        },
        Theme: "auto",
    }
}

func configPath() (string, error) {
    // XDG
    base := os.Getenv("XDG_CONFIG_HOME")
    if base == "" {
        home, err := os.UserHomeDir()
        if err != nil {
            return "", err
        }
        base = filepath.Join(home, ".config")
    }
    return filepath.Join(base, "workflow", "config.yml"), nil
}

func Load() (Config, error) {
    cfg := Default()
    path, err := configPath()
    if err != nil {
        return cfg, err
    }
    data, err := os.ReadFile(path)
    if err != nil {
        if errors.Is(err, fs.ErrNotExist) {
            return cfg, nil
        }
        return cfg, err
    }
    // Overlay: unmarshal and merge onto defaults
    var user Config
    if err := yaml.Unmarshal(data, &user); err != nil {
        return cfg, err
    }
    merge := cfg
    if len(user.Roots) > 0 { merge.Roots = user.Roots }
    if user.Depth != 0 { merge.Depth = user.Depth }
    if user.Ignore != "" { merge.Ignore = user.Ignore }
    if user.Editor.Default != "" { merge.Editor.Default = user.Editor.Default }
    if len(user.Editor.GUIFallbacks) > 0 { merge.Editor.GUIFallbacks = user.Editor.GUIFallbacks }
    if user.Terminal.Prefer != "" { merge.Terminal.Prefer = user.Terminal.Prefer }
    if user.Terminal.InPlaceShell != "" { merge.Terminal.InPlaceShell = user.Terminal.InPlaceShell }
    if user.Agents.Default != "" { merge.Agents.Default = user.Agents.Default }
    if len(user.Agents.Map) > 0 { merge.Agents.Map = user.Agents.Map }
    if len(user.Agents.Prelude) > 0 { merge.Agents.Prelude = user.Agents.Prelude }
    if user.Agents.CmdTemplate != "" { merge.Agents.CmdTemplate = user.Agents.CmdTemplate }
    if user.Theme != "" { merge.Theme = user.Theme }
    return merge, nil
}

// ExpandUser expands a path starting with ~ to the user's home.
func ExpandUser(p string) string {
    if p == "" { return p }
    if p[0] != '~' { return p }
    home, err := os.UserHomeDir()
    if err != nil { return p }
    if p == "~" { return home }
    return filepath.Join(home, p[2:])
}

