//go:build phononbundle

package phonon

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

// bundled is the pinned checkpoint and the default voice, put in bundle/
// by `make bundle` before the build.
//
//go:embed bundle
var bundled embed.FS

// Bundled reports whether this build carries the model.
const Bundled = true

// unbundle copies the files this build carries into dir where missing.
func unbundle(dir string) error {
	return fs.WalkDir(bundled, "bundle", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel("bundle", path)
		dst := filepath.Join(dir, rel)
		if _, err := os.Stat(dst); err == nil {
			return nil
		}
		data, err := bundled.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		tmp := dst + ".part"
		if err := os.WriteFile(tmp, data, 0o644); err != nil {
			return err
		}
		return os.Rename(tmp, dst)
	})
}
