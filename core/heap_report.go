package main

import (
	"fmt"
	"math"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
)

const (
	// Finer than the runtime's 512 KiB default so a 30 MB heap yields enough
	// samples to name its owners; the cost is one stack walk per 64 KiB.
	heapProfileRate = 64 << 10
	// A report travels in the support log beside the event journal.
	heapReportBytes  = 16 << 10
	heapReportSites  = 25
	heapReportFrames = 4
	// Goroutines grouped by the function they run; a connection that never
	// ended shows up here as a count that does not come back down.
	heapReportRoutines = 15
)

func init() { runtime.MemProfileRate = heapProfileRate }

func heapReport(limit int) []byte {
	// The profile describes the heap as of the last completed cycle; two
	// cycles make it the heap as it is now.
	runtime.GC()
	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)

	var out strings.Builder
	mb := func(bytes uint64) string { return fmt.Sprintf("%.1f", float64(bytes)/(1<<20)) }
	fmt.Fprintf(&out, "heap inuse=%s idle=%s released=%s objects=%d\n",
		mb(stats.HeapInuse), mb(stats.HeapIdle), mb(stats.HeapReleased), stats.HeapObjects)
	fmt.Fprintf(&out, "runtime sys=%s stacks=%s mspan=%s mcache=%s gc=%s other=%s buckhash=%s\n",
		mb(stats.Sys), mb(stats.StackInuse), mb(stats.MSpanInuse), mb(stats.MCacheInuse),
		mb(stats.GCSys), mb(stats.OtherSys), mb(stats.BuckHashSys))
	fmt.Fprintf(&out, "gc cycles=%d limit=%s goroutines=%d\n",
		stats.NumGC, mb(uint64(max(0, debug.SetMemoryLimit(-1)))), runtime.NumGoroutine())

	out.WriteString("in use by site (MB, objects):\n")
	for _, site := range heapSites() {
		fmt.Fprintf(&out, "  %6.2f %7d  %s\n", float64(site.bytes)/(1<<20), site.objects, site.stack)
	}
	out.WriteString("goroutines by function:\n")
	for _, group := range goroutineGroups() {
		fmt.Fprintf(&out, "  %5d  %s\n", group.count, group.function)
	}
	report := out.String()
	if len(report) > limit {
		report = truncateUTF8(report, limit)
	}
	return []byte(report)
}

type heapSite struct {
	stack          string
	bytes, objects int64
}

// heapSites returns the allocation sites with the most memory still in use,
// scaled up from the samples the way pprof does.
func heapSites() []heapSite {
	var records []runtime.MemProfileRecord
	n, _ := runtime.MemProfile(nil, false)
	for {
		records = make([]runtime.MemProfileRecord, n+50)
		var ok bool
		if n, ok = runtime.MemProfile(records, false); ok {
			records = records[:n]
			break
		}
	}
	bySite := map[string]*heapSite{}
	for i := range records {
		record := &records[i]
		bytes, objects := scaleHeapSample(record.InUseObjects(), record.InUseBytes(), heapProfileRate)
		if bytes <= 0 {
			continue
		}
		key := describeStack(record.Stack(), heapReportFrames)
		site := bySite[key]
		if site == nil {
			site = &heapSite{stack: key}
			bySite[key] = site
		}
		site.bytes += bytes
		site.objects += objects
	}
	sites := make([]heapSite, 0, len(bySite))
	for _, site := range bySite {
		sites = append(sites, *site)
	}
	slices.SortFunc(sites, func(a, b heapSite) int { return int(b.bytes - a.bytes) })
	return sites[:min(len(sites), heapReportSites)]
}

// scaleHeapSample estimates the true totals behind a sampled record: an
// allocation of size s is sampled with probability 1-exp(-s/rate).
func scaleHeapSample(count, size, rate int64) (int64, int64) {
	if count == 0 || size == 0 {
		return 0, 0
	}
	if rate <= 1 {
		return size, count
	}
	average := float64(size) / float64(count)
	scale := 1 / (1 - math.Exp(-average/float64(rate)))
	return int64(float64(size) * scale), int64(float64(count) * scale)
}

type goroutineGroup struct {
	function string
	count    int
}

func goroutineGroups() []goroutineGroup {
	var records []runtime.StackRecord
	n, _ := runtime.GoroutineProfile(nil)
	for {
		records = make([]runtime.StackRecord, n+50)
		var ok bool
		if n, ok = runtime.GoroutineProfile(records); ok {
			records = records[:n]
			break
		}
	}
	counts := map[string]int{}
	for i := range records {
		counts[goroutineFunction(records[i].Stack())]++
	}
	groups := make([]goroutineGroup, 0, len(counts))
	for function, count := range counts {
		groups = append(groups, goroutineGroup{function: function, count: count})
	}
	slices.SortFunc(groups, func(a, b goroutineGroup) int {
		if a.count != b.count {
			return b.count - a.count
		}
		return strings.Compare(a.function, b.function)
	})
	return groups[:min(len(groups), heapReportRoutines)]
}

// goroutineFunction names the function a goroutine was started with, then
// the one it is blocked in, which together tell a relay from a reader.
func goroutineFunction(stack []uintptr) string {
	var names []string
	frames := runtime.CallersFrames(stack)
	for {
		frame, more := frames.Next()
		if frame.Function != "" && frame.Function != "runtime.goexit" {
			names = append(names, shortFunction(frame.Function))
		}
		if !more {
			break
		}
	}
	if len(names) == 0 {
		return "?"
	}
	start := names[len(names)-1]
	for _, name := range names {
		if !strings.HasPrefix(name, "runtime.") && !strings.HasPrefix(name, "sync.") &&
			!strings.HasPrefix(name, "poll.") {
			if name == start {
				return start
			}
			return start + " @ " + name
		}
	}
	return start
}

// describeStack joins the innermost frames outside the runtime, innermost
// first, so a buffer reads as the call that asked for it.
func describeStack(stack []uintptr, limit int) string {
	var names []string
	frames := runtime.CallersFrames(stack)
	for len(names) < limit {
		frame, more := frames.Next()
		if frame.Function != "" && !strings.HasPrefix(frame.Function, "runtime.") {
			names = append(names, shortFunction(frame.Function))
		}
		if !more {
			break
		}
	}
	if len(names) == 0 {
		return "runtime"
	}
	return strings.Join(names, " < ")
}

// shortFunction drops the module path, keeping the package and function.
func shortFunction(name string) string {
	if slash := strings.LastIndexByte(name, '/'); slash >= 0 {
		return name[slash+1:]
	}
	return name
}

// truncateUTF8 cuts value to at most limit bytes without splitting a rune.
func truncateUTF8(value string, limit int) string {
	end := limit
	for end > 0 && value[end]&0xc0 == 0x80 {
		end--
	}
	return value[:end]
}
