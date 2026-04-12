//go:build !js

package futurerender

import (
	"log"
	"net/http"
	_ "net/http/pprof" // registers /debug/pprof/* handlers on DefaultServeMux
	"os"
)

// initPprof starts a pprof HTTP server if the FUTURE_CORE_PPROF environment
// variable is set to a listen address (e.g. ":6060" or "localhost:6060").
// The server runs in a background goroutine and does not block the engine.
//
// Only available on desktop builds. In WASM, initPprof is a no-op because
// the browser sandbox does not support TCP listeners — use Chrome DevTools
// → Performance tab instead.
//
// Usage:
//
//	FUTURE_CORE_PPROF=:6060 ./myprogram
//	go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30
func initPprof() {
	addr := os.Getenv("FUTURE_CORE_PPROF")
	if addr == "" {
		return
	}
	go func() {
		log.Printf("pprof: listening on %s (http://%s/debug/pprof/)", addr, addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
			log.Printf("pprof: %v", err)
		}
	}()
}
