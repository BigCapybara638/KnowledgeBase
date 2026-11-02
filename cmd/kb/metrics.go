package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type memoryMetrics struct {
	Current  uint64 `json:"currentBytes"`
	Peak     uint64 `json:"peakBytes"`
	Limit    uint64 `json:"limitBytes"`
	OOMKills uint64 `json:"oomKills"`
}

func readMetrics(dir string) (memoryMetrics, error) {
	var m memoryMetrics
	for _, field := range []struct {
		name string
		dest *uint64
	}{{"memory.current", &m.Current}, {"memory.peak", &m.Peak}, {"memory.max", &m.Limit}} {
		b, e := os.ReadFile(filepath.Join(dir, field.name))
		if e != nil {
			return m, fmt.Errorf("cgroup v2 metrics unavailable: %w", e)
		}
		s := strings.TrimSpace(string(b))
		if s == "max" && field.name == "memory.max" {
			continue
		}
		*field.dest, e = strconv.ParseUint(s, 10, 64)
		if e != nil {
			return m, e
		}
	}
	b, e := os.ReadFile(filepath.Join(dir, "memory.events"))
	if e != nil {
		return m, e
	}
	found := false
	for line := range strings.SplitSeq(string(b), "\n") {
		v := strings.Fields(line)
		if len(v) == 2 && v[0] == "oom_kill" {
			m.OOMKills, e = strconv.ParseUint(v[1], 10, 64)
			if e != nil {
				return m, e
			}
			found = true
		}
	}
	if !found {
		return m, fmt.Errorf("missing cgroup OOM counter")
	}
	return m, nil
}
