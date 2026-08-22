package cockpit

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed ui/dist/*
//go:embed ui/dist/**/*
var uiAssets embed.FS

// getUIFS returns a subtree of the embedded filesystem centered on the ui/dist folder.
func getUIFS() (http.FileSystem, error) {
	sub, err := fs.Sub(uiAssets, "ui/dist")
	if err != nil {
		return nil, err
	}
	return http.FS(sub), nil
}
