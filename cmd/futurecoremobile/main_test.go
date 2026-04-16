package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// These tests pin the CLI-peeking behavior — hasOutputFlag is the
// only pre-gomobile logic in futurecoremobile phase-0, and a false
// result there would silently let gomobile error out with a less
// helpful message. The actual bind path hands off to an external
// process and is covered by the Phase 0 integration step (real
// `gomobile bind` invocation against the futurecoreview package).

func TestHasOutputFlagDetectsDashOSpace(t *testing.T) {
	require.True(t, hasOutputFlag([]string{"-o", "out.aar", "./pkg"}))
}

func TestHasOutputFlagDetectsDashOEquals(t *testing.T) {
	require.True(t, hasOutputFlag([]string{"-o=out.aar", "./pkg"}))
}

func TestHasOutputFlagDetectsAfterOtherFlags(t *testing.T) {
	require.True(t, hasOutputFlag([]string{
		"-target=android",
		"-javapkg=com.example",
		"-o=out.aar",
		"./pkg",
	}))
}

func TestHasOutputFlagReturnsFalseWhenMissing(t *testing.T) {
	require.False(t, hasOutputFlag([]string{"-target=android", "./pkg"}))
}

func TestHasOutputFlagIgnoresDoubleDashSentinel(t *testing.T) {
	// After --, everything is treated as a positional arg even if it
	// looks like a flag. We shouldn't try to parse past it.
	require.False(t, hasOutputFlag([]string{"-target=android", "--", "-o=not-a-flag"}))
}

func TestTakesValueRecognizesGomobileFlags(t *testing.T) {
	// Anchors the list of gomobile bind flags that consume a
	// following positional argument. If gomobile adds a new
	// space-separated flag we don't list here, an unknown-flag skip
	// in hasOutputFlag may miscount — so this test documents the
	// known set.
	for _, name := range []string{
		"target", "o", "ldflags", "gcflags", "tags",
		"javapkg", "prefix", "bundleid", "iosversion",
		"androidapi", "classpath", "bootclasspath",
	} {
		require.True(t, takesValue(name), "%q should be a value-consuming flag", name)
	}
	require.False(t, takesValue("nonexistent"))
	require.False(t, takesValue("a"))
}
