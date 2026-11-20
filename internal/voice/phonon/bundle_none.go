//go:build !phononbundle

package phonon

// Bundled reports whether this build carries the model.
const Bundled = false

func unbundle(string) error { return nil }
