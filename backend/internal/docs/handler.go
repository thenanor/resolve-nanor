package docs

import (
	_ "embed"
	"net/http"

	"github.com/go-chi/chi/v5"
)

//go:embed openapi.yaml
var spec []byte

const uiPage = `<!doctype html>
<html>
<head>
	<meta charset="utf-8">
	<title>Resolve API docs</title>
	<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
	<div id="swagger-ui"></div>
	<script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
	<script>
		window.ui = SwaggerUIBundle({
			url: "/openapi.yaml",
			dom_id: "#swagger-ui",
		});
	</script>
</body>
</html>
`

type Handler struct{}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) Mount(r chi.Router) {
	r.Get("/openapi.yaml", h.spec)
	r.Get("/docs", h.ui)
}

func (h *Handler) spec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	_, _ = w.Write(spec)
}

func (h *Handler) ui(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(uiPage))
}
