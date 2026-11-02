package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMetricsReadCgroupIncludingPeakAndOOM(t *testing.T) {
	d := t.TempDir()
	for name, value := range map[string]string{"memory.current": "123\n", "memory.peak": "456\n", "memory.max": "805306368\n", "memory.events": "low 0\nhigh 0\nmax 1\noom 0\noom_kill 0\n"} {
		os.WriteFile(filepath.Join(d, name), []byte(value), 0600)
	}
	m, e := readMetrics(d)
	if e != nil || m.Current != 123 || m.Peak != 456 || m.Limit != 805306368 || m.OOMKills != 0 {
		t.Fatal(m, e)
	}
}
func TestMissingCgroupIsNotReportedAsZeroMemory(t *testing.T) {
	if _, e := readMetrics(t.TempDir()); e == nil {
		t.Fatal("missing metrics accepted")
	}
}
