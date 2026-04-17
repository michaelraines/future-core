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

func TestParseStringFlagForms(t *testing.T) {
	// Covers both "-name value" and "-name=value" forms, plus -- stop.
	cases := []struct {
		name    string
		args    []string
		flag    string
		want    string
		present bool
	}{
		{"equals form", []string{"-o=x.aar"}, "o", "x.aar", true},
		{"space form", []string{"-o", "x.aar"}, "o", "x.aar", true},
		{"double dash prefix", []string{"--o=x.aar"}, "o", "x.aar", true},
		{"javapkg equals", []string{"-javapkg=com.example.app"}, "javapkg", "com.example.app", true},
		{"missing", []string{"-target=android"}, "o", "", false},
		{"stop on --", []string{"--", "-o=x.aar"}, "o", "", false},
		{"missing value after space flag", []string{"-o"}, "o", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := parseStringFlag(c.args, c.flag)
			require.Equal(t, c.present, ok)
			require.Equal(t, c.want, got)
		})
	}
}
