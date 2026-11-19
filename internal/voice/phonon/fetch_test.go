package phonon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestEnsureFetchesOnceAndFindsVoices(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch {
		case strings.HasSuffix(r.URL.Path, "/voices.jsonl"):
			w.Write([]byte(`{"name":"Harper","filename":"voices/abc.safetensors","language":"en"}` + "\n" +
				`{"name":"Easton","filename":"voices/def.safetensors","language":"en"}` + "\n"))
		case strings.Contains(r.URL.Path, "/resolve/"+ModelRev+"/"), strings.Contains(r.URL.Path, "/resolve/"+VoicesRev+"/"):
			w.Write([]byte("data:" + r.URL.Path))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	f := Fetcher{Base: srv.URL}
	name, err := f.Ensure(context.Background(), dir, "harper")
	if err != nil {
		t.Fatal(err)
	}
	if name != "Harper" {
		t.Errorf("voice name = %q, want the catalog's Harper", name)
	}
	for _, p := range append(modelFiles, "voices/Harper.safetensors") {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("missing %s", p)
		}
	}
	b, _ := os.ReadFile(filepath.Join(dir, "voices/Harper.safetensors"))
	if !strings.Contains(string(b), "/gradium/phonon-voices/resolve/"+VoicesRev+"/voices/abc.safetensors") {
		t.Errorf("voice came from the wrong place: %s", b)
	}
	before := hits.Load()
	if _, err := f.Ensure(context.Background(), dir, "Harper"); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != before {
		t.Errorf("a second Ensure downloaded again")
	}
	if _, err := f.Ensure(context.Background(), dir, "Nobody"); err == nil || !strings.Contains(err.Error(), "Easton, Harper") {
		t.Errorf("unknown voice should list the voices: %v", err)
	}
}

func TestKeyNamesVoiceAndSource(t *testing.T) {
	a := KeyFor("", "Harper", nil)
	if a == KeyFor("", "Easton", nil) || a == KeyFor("/my/model", "Harper", nil) || a == KeyFor("", "Harper", map[string]float64{"temperature": 0.5}) {
		t.Errorf("cache keys must differ by voice, model and temperature: %s", a)
	}
}
