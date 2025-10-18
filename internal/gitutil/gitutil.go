package gitutil

import (
    "os/exec"
    "strings"
)

func RemoteURL(path string) (string, error) {
    out, err := exec.Command("git", "-C", path, "remote", "get-url", "origin").Output()
    if err != nil {
        return "", err
    }
    raw := strings.TrimSpace(string(out))
    return normalizeURL(raw), nil
}

func normalizeURL(u string) string {
    if strings.HasPrefix(u, "git@") {
        // git@github.com:user/repo.git -> https://github.com/user/repo
        u = strings.TrimPrefix(u, "git@")
        parts := strings.SplitN(u, ":", 2)
        if len(parts) == 2 {
            host, path := parts[0], parts[1]
            if strings.HasSuffix(path, ".git") { path = strings.TrimSuffix(path, ".git") }
            return "https://" + host + "/" + path
        }
    }
    if strings.HasPrefix(u, "ssh://") {
        u = strings.TrimPrefix(u, "ssh://")
        // ssh://git@host/user/repo.git -> https://host/user/repo
        if i := strings.Index(u, "@"); i >= 0 {
            u = u[i+1:]
        }
        if strings.HasSuffix(u, ".git") { u = strings.TrimSuffix(u, ".git") }
        return "https://" + u
    }
    // https or http; drop .git
    if strings.HasSuffix(u, ".git") { u = strings.TrimSuffix(u, ".git") }
    return u
}

