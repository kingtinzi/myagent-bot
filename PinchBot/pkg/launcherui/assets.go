package launcherui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed index.html
var embeddedUI embed.FS

func NewHandler(absPath string) (http.Handler, error) {
	mux := http.NewServeMux()
	RegisterConfigAPI(mux, absPath)
	RegisterAuthAPI(mux, absPath)
	RegisterAppPlatformAPI(mux, absPath)
	RegisterWorkspaceAPI(mux)
	RegisterProcessAPI(mux, absPath)

	staticFS, err := fs.Sub(embeddedUI, ".")
	if err != nil {
		return nil, err
	}
	mux.Handle("/", http.FileServer(http.FS(staticFS)))
	return mux, nil
}
