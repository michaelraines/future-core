// Command backends lists all registered Future Render backends, creates
// a device for each, queries its capabilities, and prints a summary table.
//
// Build: go build ./cmd/backends
// Run:   ./backends
package main

import (
	"fmt"
	"log"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/michaelraines/future-render/internal/backend"

	// Import all backends so their init() functions register them.
	_ "github.com/michaelraines/future-render/internal/backend/dx12"
	_ "github.com/michaelraines/future-render/internal/backend/metal"
	_ "github.com/michaelraines/future-render/internal/backend/soft"
	_ "github.com/michaelraines/future-render/internal/backend/vulkan"
	_ "github.com/michaelraines/future-render/internal/backend/webgl"
	_ "github.com/michaelraines/future-render/internal/backend/webgpu"
)

func main() {
	names := backend.Available()
	sort.Strings(names)

	fmt.Printf("Future Render — %d backend(s) registered\n\n", len(names))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "BACKEND\tMAX TEX\tMAX RT\tINSTANCED\tCOMPUTE\tMSAA\tMAX MSAA\tFP16"); err != nil {
		log.Fatal(err)
	}

	for _, name := range names {
		dev, err := backend.Create(name)
		if err != nil {
			if _, ferr := fmt.Fprintf(w, "%s\t(error: %v)\n", name, err); ferr != nil {
				log.Fatal(ferr)
			}
			continue
		}

		err = dev.Init(backend.DeviceConfig{Width: 1, Height: 1})
		if err != nil {
			if _, ferr := fmt.Fprintf(w, "%s\t(init error: %v)\n", name, err); ferr != nil {
				log.Fatal(ferr)
			}
			continue
		}

		caps := dev.Capabilities()
		if _, ferr := fmt.Fprintf(w, "%s\t%d\t%d\t%v\t%v\t%v\t%d\t%v\n",
			name,
			caps.MaxTextureSize,
			caps.MaxRenderTargets,
			caps.SupportsInstanced,
			caps.SupportsCompute,
			caps.SupportsMSAA,
			caps.MaxMSAASamples,
			caps.SupportsFloat16,
		); ferr != nil {
			log.Fatal(ferr)
		}

		dev.Dispose()
	}

	if err := w.Flush(); err != nil {
		log.Fatal(err)
	}
}
