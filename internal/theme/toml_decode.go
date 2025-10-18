package theme

import (
    toml "github.com/pelletier/go-toml/v2"
)

func decodeTOMLImpl(data []byte, out *map[string]any) error {
    return toml.Unmarshal(data, out)
}

