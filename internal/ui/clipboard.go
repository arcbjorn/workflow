package ui

import (
    "errors"
    "os/exec"
    "runtime"

    "github.com/atotto/clipboard"
)

func copyClipboard(s string) error {
    if err := clipboard.WriteAll(s); err == nil {
        return nil
    }
    // fallbacks for Wayland/X11
    if runtime.GOOS == "linux" {
        if err := exec.Command("wl-copy", s).Run(); err == nil { return nil }
        if err := exec.Command("xclip", "-selection", "clipboard").Run(); err == nil { return nil }
    }
    return errors.New("clipboard unavailable")
}

