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
	"archive/zip"
	_ "embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"
)

// Embedded Java sources — templated at overlay time with the target
// -javapkg path. The resulting .java files are compiled with `javac`
// and their .class files injected into the AAR's classes.jar.
//
//go:embed _files/FutureCoreView.java
var futureCoreViewJavaTmpl string

//go:embed _files/FutureCoreSurfaceView.java
var futureCoreSurfaceViewJavaTmpl string

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
	outPath, ok := parseOutputFlag(args)
	if !ok {
		return fmt.Errorf("-o is required; specify the output AAR (android) or xcframework (ios)")
	}

	javaPkg, _ := parseJavaPkgFlag(args)

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
	if err := cmd.Run(); err != nil {
		return err
	}

	// Only overlay Java files for Android AAR outputs. iOS gets
	// Objective-C handling in a later phase.
	if strings.HasSuffix(outPath, ".aar") {
		if javaPkg == "" {
			return fmt.Errorf("-javapkg is required for android AAR overlays (sets the package of FutureCoreView)")
		}
		if err := overlayAndroidJava(outPath, javaPkg); err != nil {
			return fmt.Errorf("java overlay: %w", err)
		}
	}
	return nil
}

// parseOutputFlag returns the -o/--o argument value, or ("", false)
// if missing. Accepts both "-o foo" and "-o=foo" forms.
func parseOutputFlag(args []string) (string, bool) {
	return parseStringFlag(args, "o")
}

// parseJavaPkgFlag returns -javapkg value (e.g. "com.example.app")
// or ("", false) if missing.
func parseJavaPkgFlag(args []string) (string, bool) {
	return parseStringFlag(args, "javapkg")
}

// parseStringFlag extracts a single string-valued flag from args
// without consuming it. Supports both "-flag value" and "-flag=value"
// forms; ignores unknown flags.
func parseStringFlag(args []string, name string) (string, bool) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" || !strings.HasPrefix(a, "-") {
			return "", false
		}
		stripped := strings.TrimLeft(a, "-")
		if strings.HasPrefix(stripped, name+"=") {
			return stripped[len(name)+1:], true
		}
		if stripped == name {
			if i+1 < len(args) {
				return args[i+1], true
			}
			return "", false
		}
	}
	return "", false
}

// hasOutputFlag kept as a thin wrapper over parseOutputFlag for
// callers that only want the presence check (tests, early-exit
// validation).
func hasOutputFlag(args []string) bool {
	_, ok := parseOutputFlag(args)
	return ok
}

// ---- Android AAR Java overlay ----
//
// gomobile bind produces an AAR that contains only the JNI trampolines
// gomobile itself generates. The futurecore Android binding layer also
// needs FutureCoreView + FutureCoreSurfaceView, which we hand-author
// under _files/. overlayAndroidJava templates those .java files with
// the user-supplied -javapkg, compiles them with javac against the
// Android SDK, and injects the resulting .class files into the AAR's
// classes.jar — all in a temp dir, so a failure in any step leaves
// the original AAR untouched.

// overlayAndroidJava mutates aarPath in place by adding compiled
// FutureCoreView / FutureCoreSurfaceView class files to its
// classes.jar entry. javaPkg is the package root (e.g.
// "com.whitewater.future"); the view classes end up under
// <javaPkg>.<pkg-basename> where pkg-basename is the directory name
// of the Go package gomobile bound (matches gomobile's own Java
// package naming convention so the view can import Futurecoreview
// from the same package).
func overlayAndroidJava(aarPath, javaPkg string) error {
	androidJar, err := findAndroidJar()
	if err != nil {
		return err
	}
	if _, err := exec.LookPath("javac"); err != nil {
		return fmt.Errorf("javac not found on PATH. Install a JDK (Temurin / Adoptium / system openjdk)")
	}

	tmp, err := os.MkdirTemp("", "futurecoremobile-overlay-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	// 1. Extract AAR into tmp/aar/, classes.jar into tmp/classes/.
	aarDir := filepath.Join(tmp, "aar")
	if err := unzip(aarPath, aarDir); err != nil {
		return fmt.Errorf("extract aar: %w", err)
	}
	classesJarPath := filepath.Join(aarDir, "classes.jar")
	classesDir := filepath.Join(tmp, "classes")
	if err := unzip(classesJarPath, classesDir); err != nil {
		return fmt.Errorf("extract classes.jar: %w", err)
	}

	// 2. Template Java source files and write to tmp/src/<pkgPath>/.
	// Note: gomobile maps Go package "futurecoreview" to Java package
	// "<javaPkg>.futurecoreview"; our view classes live in the same
	// package so `import ...Futurecoreview` resolves without qualified
	// class names in the .java source.
	prefixLower := "futurecoreview"
	javaPkgPath := strings.ReplaceAll(javaPkg, ".", "/") + "/" + prefixLower
	srcDir := filepath.Join(tmp, "src", javaPkgPath)
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		return err
	}
	tmplData := map[string]string{
		"JavaPkg":     javaPkg,
		"PrefixLower": prefixLower,
	}
	if err := writeTemplatedFile(filepath.Join(srcDir, "FutureCoreView.java"), futureCoreViewJavaTmpl, tmplData); err != nil {
		return err
	}
	if err := writeTemplatedFile(filepath.Join(srcDir, "FutureCoreSurfaceView.java"), futureCoreSurfaceViewJavaTmpl, tmplData); err != nil {
		return err
	}

	// 3. Compile with javac. Classpath includes the gomobile-generated
	// classes.jar (for the Futurecoreview trampoline class we import)
	// and android.jar (for Android framework types).
	classesOut := filepath.Join(tmp, "compiled")
	if err := os.MkdirAll(classesOut, 0o755); err != nil {
		return err
	}
	javac := exec.Command("javac",
		"-d", classesOut,
		"-source", "1.8", "-target", "1.8",
		"-cp", androidJar+string(os.PathListSeparator)+classesJarPath,
		filepath.Join(srcDir, "FutureCoreSurfaceView.java"),
		filepath.Join(srcDir, "FutureCoreView.java"),
	)
	javac.Stdout = os.Stdout
	javac.Stderr = os.Stderr
	if err := javac.Run(); err != nil {
		return fmt.Errorf("javac: %w", err)
	}

	// 4. Merge compiled .class files into classes/.
	if err := copyTree(classesOut, classesDir); err != nil {
		return fmt.Errorf("merge compiled classes: %w", err)
	}

	// 5. Rebuild classes.jar and the AAR.
	if err := zipDir(classesDir, classesJarPath); err != nil {
		return fmt.Errorf("rebuild classes.jar: %w", err)
	}
	if err := zipDir(aarDir, aarPath); err != nil {
		return fmt.Errorf("rebuild aar: %w", err)
	}
	return nil
}

// findAndroidJar returns a path to an android.jar from the installed
// Android SDK, preferring the highest platform version available.
func findAndroidJar() (string, error) {
	sdk := os.Getenv("ANDROID_HOME")
	if sdk == "" {
		sdk = os.Getenv("ANDROID_SDK_ROOT")
	}
	if sdk == "" {
		return "", fmt.Errorf("ANDROID_HOME/ANDROID_SDK_ROOT not set; needed to locate android.jar")
	}
	platforms := filepath.Join(sdk, "platforms")
	entries, err := os.ReadDir(platforms)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", platforms, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "android-") {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return "", fmt.Errorf("no android-NN platforms under %s; install with sdkmanager \"platforms;android-34\"", platforms)
	}
	sort.Strings(names) // android-19 < android-21 < ... < android-34 textually for common versions
	for i := len(names) - 1; i >= 0; i-- {
		p := filepath.Join(platforms, names[i], "android.jar")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("no android.jar found under %s", platforms)
}

// writeTemplatedFile renders a text/template body with the given
// data and writes it to path.
func writeTemplatedFile(path, body string, data any) error {
	t, err := template.New(filepath.Base(path)).Parse(body)
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return t.Execute(f, data)
}

// unzip extracts src into dst, preserving the directory structure.
func unzip(src, dst string) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		outPath := filepath.Join(dst, f.Name) //nolint:gosec // trusted archive produced in the same pipeline
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(outPath, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		w, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}
		_, copyErr := io.Copy(w, rc) //nolint:gosec // trusted archive
		rc.Close()
		if closeErr := w.Close(); closeErr != nil && copyErr == nil {
			copyErr = closeErr
		}
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

// zipDir packs dir's contents (not dir itself) into a zip at path.
// File timestamps are zeroed so re-builds are reproducible.
func zipDir(dir, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	err = filepath.Walk(dir, func(p string, info os.FileInfo, werr error) error {
		if werr != nil {
			return werr
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if info.IsDir() {
			// zip doesn't require explicit directory entries for files
			// beneath them, so we only add non-empty subdirectories if
			// needed. Skip to keep the archive minimal.
			return nil
		}
		hdr, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		hdr.Method = zip.Deflate
		hdr.Modified = time.Time{}
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		src, err := os.Open(p)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(w, src)
		src.Close()
		return copyErr
	})
	if err != nil {
		zw.Close()
		return err
	}
	return zw.Close()
}

// copyTree copies every file under src into dst, preserving the
// relative path layout.
func copyTree(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		r, err := os.Open(p)
		if err != nil {
			return err
		}
		defer r.Close()
		w, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		defer w.Close()
		_, err = io.Copy(w, r)
		return err
	})
}
