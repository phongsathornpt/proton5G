package template

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
)

// PageData is optional template data for the shell.
type PageData struct {
	Title   string
	Version string
}

var (
	indexOnce sync.Once
	indexTmpl *template.Template
	indexErr  error
	indexHTML []byte
)

func loadTemplates() {
	indexTmpl = template.New("root")
	err := fs.WalkDir(Assets, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".html") {
			return nil
		}
		b, err := Assets.ReadFile(path)
		if err != nil {
			return err
		}
		// Parse each file into the set (defines accumulate).
		name := filepath.ToSlash(path)
		if _, err := indexTmpl.New(name).Parse(string(b)); err != nil {
			return fmt.Errorf("parse %s: %w", name, err)
		}
		return nil
	})
	if err != nil {
		indexErr = err
		return
	}
	// Prefer executing the "base" define.
	var buf bytes.Buffer
	if err := indexTmpl.ExecuteTemplate(&buf, "base", PageData{Title: "FM350 Manager"}); err != nil {
		indexErr = fmt.Errorf("execute base: %w", err)
		return
	}
	indexHTML = buf.Bytes()
}

// RenderIndex returns the composed HTML document for GET /.
func RenderIndex() ([]byte, error) {
	indexOnce.Do(loadTemplates)
	if indexErr != nil {
		return nil, indexErr
	}
	// Return a copy so callers cannot mutate the cache.
	out := make([]byte, len(indexHTML))
	copy(out, indexHTML)
	return out, nil
}
