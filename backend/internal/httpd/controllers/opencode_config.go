package controllers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/httpd/apispec"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/httpd/envelope"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/service/opencodeconfig"
)

// OpencodeConfigService reads and writes the user's own opencode configuration.
type OpencodeConfigService interface {
	Read(ctx context.Context) (opencodeconfig.Document, error)
	Write(ctx context.Context, content string) (opencodeconfig.Document, error)
}

// OpencodeConfigController serves the Tools settings surface.
type OpencodeConfigController struct {
	Svc OpencodeConfigService
}

// Register mounts the opencode config routes.
func (c *OpencodeConfigController) Register(r chi.Router) {
	r.Get("/settings/opencode-config", c.get)
	r.Put("/settings/opencode-config", c.put)
}

func (c *OpencodeConfigController) get(w http.ResponseWriter, r *http.Request) {
	if c.Svc == nil {
		apispec.NotImplemented(w, r, "GET", "/api/v1/settings/opencode-config")
		return
	}
	doc, err := c.Svc.Read(r.Context())
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusOK, doc)
}

// put replaces the config with the user's text.
//
// The handler decodes the body itself rather than binding a struct so a body it
// cannot understand produces the JSONC error the service raised, instead of a
// generic decode failure the editor cannot show.
func (c *OpencodeConfigController) put(w http.ResponseWriter, r *http.Request) {
	if c.Svc == nil {
		apispec.NotImplemented(w, r, "PUT", "/api/v1/settings/opencode-config")
		return
	}
	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		envelope.WriteAPIError(w, r, http.StatusBadRequest, "invalid_request",
			"INVALID_OPENCODE_CONFIG_BODY", "request body must be JSON with a content string", nil)
		return
	}
	doc, err := c.Svc.Write(r.Context(), body.Content)
	if err != nil {
		// 422 rather than 400: the request was understood, the document it
		// carried was not. The editor shows this message beside the text.
		envelope.WriteAPIError(w, r, http.StatusUnprocessableEntity, "invalid_request",
			"INVALID_OPENCODE_CONFIG", err.Error(), nil)
		return
	}
	envelope.WriteJSON(w, http.StatusOK, doc)
}
