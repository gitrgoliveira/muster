package services

import "github.com/gitrgoliveira/muster/internal/ws"

// Publisher is the function signature for broadcasting WebSocket events.
// Decouples the services layer from hub/client internals — only the Frame
// type is shared.
type Publisher func(frame ws.Frame)
