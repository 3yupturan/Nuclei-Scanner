package scripts

import (
	"embed"
	_ "embed"
	"path/filepath"

	"github.com/dop251/goja"
)

//go:embed js
var embedFS embed.FS

// RegisterScripts registers all scripts in the runtime
func RegisterScripts(runtime *goja.Runtime) error {
	dirs, err := embedFS.ReadDir("js")
	if err != nil {
		return err
	}
	for _, dir := range dirs {
		if dir.IsDir() {
			continue
		}
		contents, err := embedFS.ReadFile(filepath.Join("js", dir.Name()))
		if err != nil {
			return err
		}
		_, err = runtime.RunString(string(contents))
		if err != nil {
			return err
		}
	}
	return nil
}
