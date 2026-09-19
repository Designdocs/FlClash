package main

import (
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"
)

var heapReportKeep [][]byte

//go:noinline
func holdHeapReportBuffers() {
	for i := 0; i < 64; i++ {
		heapReportKeep = append(heapReportKeep, make([]byte, 64<<10))
	}
}

func TestHeapReportNamesTheSitesThatHoldMemory(t *testing.T) {
	holdHeapReportBuffers()
	defer func() { heapReportKeep = nil }()
	report := string(heapReport(heapReportBytes))
	for _, section := range []string{"heap inuse=", "runtime sys=", "gc cycles=", "in use by site", "goroutines by function:"} {
		if !strings.Contains(report, section) {
			t.Fatalf("missing %q in:\n%s", section, report)
		}
	}
	if !strings.Contains(report, "holdHeapReportBuffers") {
		t.Fatalf("the 4 MB the test holds is not named:\n%s", report)
	}
	if strings.Contains(report, "/") && strings.Contains(report, "github.com") {
		t.Fatalf("module paths leaked:\n%s", report)
	}
	runtime.KeepAlive(heapReportKeep)
}

func TestHeapReportStaysWithinItsLimit(t *testing.T) {
	report := heapReport(200)
	if len(report) > 200 || !utf8.Valid(report) {
		t.Fatalf("report of %d bytes", len(report))
	}
}

func TestScaleHeapSample(t *testing.T) {
	if bytes, objects := scaleHeapSample(0, 0, heapProfileRate); bytes != 0 || objects != 0 {
		t.Fatal("empty record scaled")
	}
	// Allocations far larger than the rate are always sampled.
	if bytes, objects := scaleHeapSample(2, 32<<20, heapProfileRate); bytes != 32<<20 || objects != 2 {
		t.Fatalf("large allocations rescaled: %d %d", bytes, objects)
	}
	// Small ones are sampled rarely, so each sample stands for many.
	if bytes, _ := scaleHeapSample(1, 64, heapProfileRate); bytes < 60<<10 {
		t.Fatalf("small sample not scaled up: %d", bytes)
	}
}

func TestShortFunctionDropsTheModulePath(t *testing.T) {
	if got := shortFunction("github.com/metacubex/mihomo/tunnel.handleTCPConn"); got != "tunnel.handleTCPConn" {
		t.Fatal(got)
	}
}
