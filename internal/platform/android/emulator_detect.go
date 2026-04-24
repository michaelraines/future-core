//go:build android

package android

import (
	"os/exec"
	"strings"
	"sync"
)

// IsRanchuEmulator reports whether the current Android device is the
// qemu/goldfish/ranchu-based Android emulator as opposed to a real
// device. We use it to force a non-Vulkan rendering path because
// gfxstream's guest Vulkan driver has a QSRI sync-fd race that hangs
// any post-submit sync primitive (Vulkan works correctly on real
// Android GPUs; this workaround is emulator-only).
//
// Detection invokes /system/bin/getprop to read a handful of
// emulator-fingerprint props. Since API 29 these live split across
// /system, /vendor, /product, /odd_*, and parsing the files by hand
// would miss some layouts — getprop reads the merged view. The
// binary is tiny (<40 KB) and returns quickly (~1-5 ms), and we
// only call it once at init and cache.
//
// Checked in order of specificity:
//   - ro.hardware.vulkan=ranchu  — only set by the emulator's Vulkan
//     stub. Strongest signal; only present on AVDs with Vulkan, but
//     those are exactly the AVDs where our Vulkan backend breaks.
//   - ro.kernel.qemu=1            — set by qemu's kernel command line
//     on every ranchu/goldfish AVD regardless of Vulkan availability.
//   - ro.hardware=ranchu|goldfish — classic emulator fingerprint.
//   - ro.boot.hardware=ranchu     — kernel-boot equivalent.
//
// Safe for concurrent use; cached on first call.
func IsRanchuEmulator() bool {
	emuDetectOnce.Do(detectEmulator)
	return emuDetectResult
}

var (
	emuDetectOnce   sync.Once
	emuDetectResult bool
)

func detectEmulator() {
	checks := []struct {
		prop  string
		match func(string) bool
	}{
		{"ro.hardware.vulkan", func(v string) bool { return v == "ranchu" }},
		{"ro.kernel.qemu", func(v string) bool { return v == "1" }},
		{"ro.hardware", func(v string) bool { return v == "ranchu" || v == "goldfish" }},
		{"ro.boot.hardware", func(v string) bool { return v == "ranchu" || v == "goldfish" }},
		{"ro.hardware.egl", func(v string) bool { return v == "emulation" }},
	}
	for _, c := range checks {
		out, err := exec.Command("/system/bin/getprop", c.prop).Output()
		if err != nil {
			continue
		}
		val := strings.TrimSpace(string(out))
		if c.match(val) {
			emuDetectResult = true
			return
		}
	}
}
