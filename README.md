# workflow — fast repo overview TUI

Fast Bubble Tea TUI to see all your repos, their state, and launch common actions (editors, terminals, git UI, tasks, agents). Follows your Omarchy theme.

Build
- GOCACHE=$PWD/.cache/go-build go build -o ./workflow ./cmd/workflow
```

```
GOCACHE=$PWD/.cache/go-build go build -o ./workflow ./cmd/workflow && ./workflow
```
```

Run
- ./workflow

Config
- Location: ~/.config/workflow/config.yml
- Example:
  roots: ["~/projects", "~/tools"]
  depth: 2
  editor:
    default: nvim
    gui_fallbacks: [cursor, code]
  terminal:
    prefer: alacritty
  agents:
    default: claude
    map:
      claude: claude
      gemini: gemini
      codex: openai chat
      opencode: opencode

Keys
- j/k, arrows navigate; Enter details; / filter; R refresh; ? help; q quit
- m group; x expand; s/S sort; l lazygit; f fetch
- e nvim (new window); E GUI editor; o new shell window
- r tasks picker (table); r open README (details)
- b open README (new window via bat/less)
- a agent picker; A launch default agent
- y copy path; u open remote URL; Y copy remote URL

Notes
- Theme: auto-follows Omarchy current theme (~/.config/omarchy/current/theme) with live updates
- README opens in a new terminal using bat/batcat (fallback less) for speed
- Discovery cache: ~/.local/state/workflow/cache.json (TTL configurable)
- Tip (Arch): pacman -S bat for best README viewing
