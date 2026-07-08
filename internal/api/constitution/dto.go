package constitution

import (
	"time"

	"github.com/gitrgoliveira/muster/internal/services"
)

// getResponse is the wire shape of GET/PUT /api/v1/constitution.
type getResponse struct {
	Markdown  string     `json:"markdown"`
	Version   int        `json:"version"`
	UpdatedAt *time.Time `json:"updatedAt"` // null on a fresh install (never set)
}

// putRequest is the body of PUT /api/v1/constitution.
type putRequest struct {
	Markdown string `json:"markdown"`
}

func toResponse(c services.Constitution) getResponse {
	resp := getResponse{Markdown: c.Markdown, Version: c.Version}
	if !c.UpdatedAt.IsZero() {
		t := c.UpdatedAt
		resp.UpdatedAt = &t
	}
	return resp
}
