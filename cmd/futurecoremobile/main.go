// futurecoremobile is a thin wrapper around `gomobile bind` that
// produces an Android AAR (and, in later phases, an iOS xcframework)
// for a future-core-powered mobile library.
//
// Phase 0 scope: forward CLI args to `gomobile bind` and ensure the
// github.com/michaelraines/future-core/mobile/futurecoreview package
// is always included in the bind. This package holds the JNI-callable
// trampolines the host Activity invokes. Users who import this AAR
// can then call the exposed Java functions (once Phase 1 adds the
// FutureCoreView.java wrapper class) without knowing that
// futurecoreview is the underlying Go package.
//
// Later phases will extend this tool to:
//   - Compile and overlay FutureCoreView.java / FutureCoreSurfaceView.java
//     (authored here, clean-room, under cmd/futurecoremobile/_files/)
//     into the AAR's classes.jar.
//   - Rewrite the gomobile-generated Java package names to match the
//     user-supplied -javapkg, analogous to what ebitenmobile does
//     but using our own template substitution.
//   - Add an iOS xcframework path that embeds FutureCoreView.m / .h.
//
// ebitenmobile is the reference for WHAT needs to happen at each
// phase; none of its code is copied here.
//
// Usage:
//
//	futurecoremobile bind -target=android -javapkg=com.example.app -o out.aar ./mobile/mygame
//
// Install:
//
//	go install github.com/michaelraines/future-core/cmd/futurecoremobile@latest
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const (
	cmdName = "futurecoremobile"

	// futurecoreviewPkg is the Go package whose exported functions
	// form the JNI-callable surface. gomobile bind will include it
	// automatically so the host Java side can call into the engine
	// without the user listing it on the command line.
	futurecoreviewPkg = "github.com/michaelraines/future-core/mobile/futurecoreview"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "bind":
		if err := bind(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", cmdName, err)
			os.Exit(1)
		}
	case "-h", "--help", "help":
		usage()
	case "version":
		fmt.Println("futurecoremobile phase-0 (gomobile wrapper)")
	default:
		fmt.Fprintf(os.Stderr, "%s: unknown subcommand %q\n\n", cmdName, os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: %s bind [gomobile-bind-flags] PACKAGE

%s wraps `+"`gomobile bind`"+` for future-core-backed Android/iOS
libraries. It forwards all flags unchanged and ensures the
futurecoreview JNI-bridge package is always included in the bind.

All flags accepted by `+"`gomobile bind`"+` are passed through:
  -target=android[/arch,...]  -javapkg=...  -o=...
  -ldflags=...  -tags=...     -prefix=...
  -bundleid=...  -iosversion=...  -androidapi=...

See "gomobile bind -help" for the full list.
`, cmdName, cmdName)
}

func bind(args []string) error {
	if _, err := exec.LookPath("gomobile"); err != nil {
		return fmt.Errorf("gomobile not found on PATH. Install with: go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init")
	}

	// A user's explicit -o target is mandatory (mirrors gomobile's
	// requirement). Catch a missing -o early so the error message is
	// useful rather than a raw gomobile failure.
	if !hasOutputFlag(args) {
		return fmt.Errorf("-o is required; specify the output AAR (android) or xcframework (ios)")
	}

	// Append the JNI-bridge package to whatever the user asked for.
	// gomobile bind accepts multiple packages; ours is additive and
	// doesn't clash with user packages because its exported surface
	// has no name overlap.
	bindArgs := append([]string{"bind"}, args...)
	bindArgs = append(bindArgs, futurecoreviewPkg)

	cmd := exec.Command("gomobile", bindArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	return cmd.Run()
}

// hasOutputFlag reports whether -o or --o appears among the passed
// flag arguments. Accepts both "-o foo" and "-o=foo" forms.
func hasOutputFlag(args []string) bool {
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	fs.SetOutput(newDiscardWriter())
	var out string
	fs.StringVar(&out, "o", "", "")
	// gomobile's flagset includes many flags we don't parse here —
	// ignore unknown flags so our peek doesn't fail on them.
	ignoreUnknownFlags(fs, args)
	return out != ""
}

// ignoreUnknownFlags parses the given args through fs but silences
// "flag provided but not defined" errors. Stops at the first
// non-flag argument or -- sentinel.
func ignoreUnknownFlags(fs *flag.FlagSet, args []string) {
	for len(args) > 0 {
		a := args[0]
		if a == "--" || !strings.HasPrefix(a, "-") {
			return
		}
		// Strip leading dashes and split on '=' for --foo=bar form.
		name := strings.TrimLeft(a, "-")
		eq := strings.Index(name, "=")
		var hasValue bool
		if eq >= 0 {
			name = name[:eq]
			hasValue = true
		}
		if fl := fs.Lookup(name); fl != nil {
			// Defer to the real flag parser for one-arg consumption.
			if err := fs.Parse(args); err == nil {
				return
			}
			// Parse failed — fall through and skip manually.
		}
		// Unknown flag: skip it. If no '=' was present we'd normally
		// consume the next arg as its value, but for our purposes
		// (peeking for -o) a false negative on other flags is fine.
		if hasValue || !takesValue(name) {
			args = args[1:]
		} else {
			args = args[1:]
			if len(args) > 0 {
				args = args[1:]
			}
		}
	}
}

// takesValue returns true for gomobile bind flags that consume a
// following argument in the "-flag value" (not "-flag=value") form.
// Used only to advance past unknown flags correctly.
var takesValue = func(name string) bool {
	switch name {
	case "target", "o", "ldflags", "gcflags", "tags",
		"javapkg", "prefix", "bundleid", "iosversion",
		"androidapi", "classpath", "bootclasspath":
		return true
	}
	return false
}

// newDiscardWriter returns an io.Writer that silently drops writes.
// Used to silence the flag package's default error/usage output when
// we're only peeking at args.
type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

func newDiscardWriter() discardWriter { return discardWriter{} }
