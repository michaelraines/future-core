//go:build js

package futurerender

// initPprof is a no-op in WASM builds. The browser sandbox does not
// support TCP listeners, so net/http/pprof cannot serve profiles.
// Use Chrome DevTools → Performance tab for browser-side profiling.
func initPprof() {}
