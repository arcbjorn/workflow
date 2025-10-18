package run

import (
    "fmt"
    "os/exec"
    "strings"

    "workflow/internal/config"
)

func OpenGUIEditor(cwd string, cfg config.Config) error {
    editors := append([]string{}, cfg.Editor.GUIFallbacks...)
    // try preferred GUI first if set explicitly in default and looks like GUI
    if cfg.Editor.Default == "cursor" || cfg.Editor.Default == "code" {
        editors = append([]string{cfg.Editor.Default}, editors...)
    }
    seen := map[string]struct{}{}
    for _, e := range editors {
        if e == "" { continue }
        if _, ok := seen[e]; ok { continue }
        seen[e] = struct{}{}
        if _, err := exec.LookPath(e); err == nil {
            // cursor/code both accept a path argument
            cmd := exec.Command(e, cwd)
            return cmd.Start()
        }
    }
    return fmt.Errorf("no GUI editor found (tried: %s)", strings.Join(editors, ", "))
}

