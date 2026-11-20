// Package phonon is a local voice: Gradium's Phonon 31M English model
// (huggingface.co/gradium/phonon-31m-en, Apache-2.0), run on the CPU by a
// small Rust wrapper around the ptts runtime that cgo links into reel.
//
// Release builds carry the pinned checkpoint and the Harper voice inside
// the binary (see bundle.go) and copy them into the cache on first use.
// Other builds, and other voices, download them into the cache on the
// first narrated render, and after that everything runs offline. model= on the voice line points at a
// checkpoint folder of your own instead.
package phonon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ErrNotBuilt is returned by New in builds without the local voice.
var ErrNotBuilt = errors.New("this reel was built without the local Phonon voice; build it with `make build`, or render with --mute")

// The checkpoint and voice catalog, pinned to the revisions ptts 0.7.1 uses.
const (
	ModelRepo  = "gradium/phonon-31m-en"
	ModelRev   = "92741c5b07d085ff840e26fea8ce43110b87852f"
	VoicesRepo = "gradium/phonon-voices"
	VoicesRev  = "da6c9ec0d47be8df5c87c730a3aa97187f3229c3"
)

// modelFiles are what the runtime reads from a checkpoint folder.
var modelFiles = []string{"config.json", "tokenizer.json", "model.q8.gguf", "default-voice.safetensors"}

// DefaultDir is where the pinned checkpoint is kept.
func DefaultDir() string {
	d, err := os.UserCacheDir()
	if err != nil {
		d = os.TempDir()
	}
	return filepath.Join(d, "reel", "phonon", "31m-en@"+ModelRev[:8])
}

// Fetcher downloads the checkpoint and voices it is missing.
type Fetcher struct {
	Base   string // https://huggingface.co; tests point it at a local server
	Client *http.Client
	Log    func(string)
}

func (f Fetcher) base() string {
	if f.Base != "" {
		return f.Base
	}
	return "https://huggingface.co"
}

func (f Fetcher) client() *http.Client {
	if f.Client != nil {
		return f.Client
	}
	return http.DefaultClient
}

// Ensure makes sure dir holds the checkpoint and, when voice is not empty,
// that voice, and returns the voice's name as the runtime knows it.
func (f Fetcher) Ensure(ctx context.Context, dir, voice string) (string, error) {
	if err := unbundle(dir); err != nil {
		return "", err
	}
	for _, name := range modelFiles {
		if err := f.get(ctx, ModelRepo, ModelRev, name, filepath.Join(dir, name)); err != nil {
			return "", err
		}
	}
	if voice == "" || strings.EqualFold(voice, "default") {
		return "default", nil
	}
	if _, err := os.Stat(filepath.Join(dir, "voices", voice+".safetensors")); err == nil {
		return voice, nil
	}
	catalog := filepath.Join(dir, "voices.jsonl")
	if err := f.get(ctx, VoicesRepo, VoicesRev, "voices.jsonl", catalog); err != nil {
		return "", err
	}
	entries, err := readCatalog(catalog)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if strings.EqualFold(e.Name, voice) {
			dst := filepath.Join(dir, "voices", e.Name+".safetensors")
			return e.Name, f.get(ctx, VoicesRepo, VoicesRev, e.Filename, dst)
		}
	}
	return "", fmt.Errorf("phonon has no voice %q; try one of %s", voice, strings.Join(Names(entries, "en"), ", "))
}

// CatalogVoice is one entry of the voice catalog.
type CatalogVoice struct {
	Name        string `json:"name"`
	Filename    string `json:"filename"`
	Language    string `json:"language"`
	Gender      string `json:"gender"`
	Description string `json:"description"`
}

func readCatalog(path string) ([]CatalogVoice, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	var out []CatalogVoice
	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var v CatalogVoice
		if json.Unmarshal(sc.Bytes(), &v) == nil && v.Name != "" && v.Filename != "" {
			out = append(out, v)
		}
	}
	return out, sc.Err()
}

// Names lists the voices for a language, sorted.
func Names(entries []CatalogVoice, lang string) []string {
	var out []string
	for _, e := range entries {
		if lang == "" || e.Language == lang {
			out = append(out, e.Name)
		}
	}
	sort.Strings(out)
	return out
}

// get downloads one file of a repo at a revision unless dst exists.
func (f Fetcher) get(ctx context.Context, repo, rev, name, dst string) error {
	if _, err := os.Stat(dst); err == nil {
		return nil
	}
	if f.Log != nil {
		f.Log(fmt.Sprintf("downloading %s from %s", name, repo))
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	url := fmt.Sprintf("%s/%s/resolve/%s/%s", f.base(), repo, rev, name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := f.client().Do(req)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading %s: %s", name, resp.Status)
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".part-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return fmt.Errorf("downloading %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dst)
}

// KeyFor is the cache key for a voice of a checkpoint.
func KeyFor(model, voice string, tune map[string]float64) string {
	src := "31m-en@" + ModelRev[:8]
	if model != "" {
		src = "local:" + model
	}
	k := "phonon/" + src + "/" + strings.ToLower(voice)
	if t, ok := tune["temperature"]; ok {
		k += fmt.Sprintf("/temperature=%g", t)
	}
	return k + "/pcm24k"
}
