// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"
)

// Stat kinds recorded per repository and group. The string values match the
// keys used in the on-disk stats file shared with the Python implementation.
const (
	statView            = "view"
	statFetch           = "fetch"
	statPush            = "push"
	statDownload        = "download"
	statReleaseDownload = "release_download"
)

var statKinds = []string{statView, statFetch, statPush, statDownload, statReleaseDownload}

// statsPersistInterval matches the Python 180 second persistence cadence.
const statsPersistInterval = 180 * time.Second

// statsLookbackDays is the default window the stats page renders.
const statsLookbackDays = 90

// statSeries aggregates one kind over a day range.
type statSeries struct {
	Daily   []int64
	Total   int64
	Peak    int64
	PeakDay string
}

// repoStats is the aggregated render model for the stats page.
type repoStats struct {
	Days           []string
	DayLabels      []string
	LookbackDays   int
	ActualDays     int
	Series         map[string]*statSeries
	DownloadsTotal *statSeries
	ActivityScore  int
	ActivityLevel  string
	DateRange      string
}

// statsDB mirrors the msgpack layout the Python node persists:
// {"pages":{"front":{day:count}}, "groups":{group:{"view":{day:count},
// "repositories":{repo:{kind:{day:count}}}}}}
type statsDB struct {
	Pages  map[string]map[string]int64 `msgpack:"pages"`
	Groups map[string]map[string]any   `msgpack:"groups"`
}

// statsStore records and persists daily counters.
type statsStore struct {
	mu      sync.Mutex
	path    string
	dirty   bool
	pages   map[string]map[string]int64
	groups  map[string]map[string]map[string]int64
	repos   map[string]map[string]map[string]map[string]int64
	stop    chan struct{}
	stopped sync.WaitGroup
}

func todayKey() string { return time.Now().Format("2006-01-02") }

func newStatsStore(path string) *statsStore {
	s := &statsStore{
		path:   path,
		pages:  map[string]map[string]int64{},
		groups: map[string]map[string]map[string]int64{},
		repos:  map[string]map[string]map[string]map[string]int64{},
		stop:   make(chan struct{}),
	}
	s.load()
	return s
}

// load reads the persisted stats file if present. It tolerates the nested
// msgpack layout written by the Python node.
func (s *statsStore) load() {
	if s.path == "" {
		return
	}
	b, err := os.ReadFile(s.path) // #nosec G304 -- operator stats file
	if err != nil {
		return
	}
	var raw map[string]any
	if err := msgpack.Unmarshal(b, &raw); err != nil {
		return
	}
	if pages, ok := raw["pages"].(map[string]any); ok {
		for page, days := range pages {
			s.pages[page] = int64Map(days)
		}
	}
	if groups, ok := raw["groups"].(map[string]any); ok {
		for gname, gval := range groups {
			gm, ok := gval.(map[string]any)
			if !ok {
				continue
			}
			g := map[string]map[string]int64{}
			for kind, dv := range gm {
				if kind == "repositories" {
					if repos, ok := dv.(map[string]any); ok {
						if s.repos[gname] == nil {
							s.repos[gname] = map[string]map[string]map[string]int64{}
						}
						for rname, rval := range repos {
							rm, ok := rval.(map[string]any)
							if !ok {
								continue
							}
							r := map[string]map[string]int64{}
							for k, dvv := range rm {
								r[k] = int64Map(dvv)
							}
							s.repos[gname][rname] = r
						}
					}
					continue
				}
				g[kind] = int64Map(dv)
			}
			s.groups[gname] = g
		}
	}
}

// int64Map converts a decoded msgpack day-count map to int64 values.
func int64Map(v any) map[string]int64 {
	out := map[string]int64{}
	m, ok := v.(map[string]any)
	if !ok {
		if mm, ok2 := v.(map[any]any); ok2 {
			for k, val := range mm {
				out[fmt.Sprint(k)] = toInt64(val)
			}
		}
		return out
	}
	for k, val := range m {
		out[k] = toInt64(val)
	}
	return out
}

func toInt64(v any) int64 {
	switch x := v.(type) {
	case int:
		return int64(x)
	case int8:
		return int64(x)
	case int16:
		return int64(x)
	case int32:
		return int64(x)
	case int64:
		return x
	case uint8:
		return int64(x)
	case uint16:
		return int64(x)
	case uint32:
		return int64(x)
	case uint64:
		if x > math.MaxInt64 {
			return math.MaxInt64
		}
		return int64(x)
	case float64:
		return int64(x)
	default:
		return 0
	}
}

// persist writes the stats file atomically when dirty.
func (s *statsStore) persist() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.persistLocked()
}

func (s *statsStore) persistLocked() {
	if s.path == "" || !s.dirty {
		return
	}
	groups := map[string]any{}
	for gname, g := range s.groups {
		gm := map[string]any{}
		for kind, days := range g {
			gm[kind] = days
		}
		groups[gname] = gm
	}
	// Repository-only records may not have a group entry, so merge the repo
	// map over the groups seen above.
	for gname, repos := range s.repos {
		gm, ok := groups[gname].(map[string]any)
		if !ok {
			gm = map[string]any{}
			groups[gname] = gm
		}
		gm["repositories"] = repos
	}
	for _, gm := range groups {
		if m, ok := gm.(map[string]any); ok && m["repositories"] == nil {
			m["repositories"] = map[string]any{}
		}
	}
	blob, err := msgpack.Marshal(map[string]any{"pages": s.pages, "groups": groups})
	if err != nil {
		return
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, blob, 0o600); err != nil {
		return
	}
	if err := os.Rename(tmp, s.path); err == nil {
		s.dirty = false
	}
}

// run persists every statsPersistInterval until closed.
func (s *statsStore) run() {
	s.stopped.Add(1)
	go func() {
		defer s.stopped.Done()
		t := time.NewTicker(statsPersistInterval)
		defer t.Stop()
		for {
			select {
			case <-s.stop:
				s.persist()
				return
			case <-t.C:
				s.persist()
			}
		}
	}()
}

func (s *statsStore) close() {
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
	s.stopped.Wait()
}

// record increments the counter for page, group, or repository scope.
func (s *statsStore) record(group, repo, kind string) {
	if s == nil {
		return
	}
	day := todayKey()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dirty = true
	switch {
	case group == "":
		if s.pages["front"] == nil {
			s.pages["front"] = map[string]int64{}
		}
		s.pages["front"][day]++
	case repo == "":
		if s.groups[group] == nil {
			s.groups[group] = map[string]map[string]int64{}
		}
		if s.groups[group][kind] == nil {
			s.groups[group][kind] = map[string]int64{}
		}
		s.groups[group][kind][day]++
	default:
		if s.repos[group] == nil {
			s.repos[group] = map[string]map[string]map[string]int64{}
		}
		if s.repos[group][repo] == nil {
			s.repos[group][repo] = map[string]map[string]int64{}
		}
		if s.repos[group][repo][kind] == nil {
			s.repos[group][repo][kind] = map[string]int64{}
		}
		s.repos[group][repo][kind][day]++
	}
}

// ignored reports whether remote is in an ignore list by hash.
func (n *Node) statsIgnored(remote *identity.Identity) bool {
	if remote == nil {
		return false
	}
	return n.cfg.StatsIgnoreIdentities[strings.ToLower(fmt.Sprintf("%x", remote.Hash()))]
}

func (n *Node) statsPushIgnored(remote *identity.Identity) bool {
	if remote == nil {
		return false
	}
	return n.cfg.StatsPushIgnoreIdentities[strings.ToLower(fmt.Sprintf("%x", remote.Hash()))]
}

// viewSucceeded records a page view event, matching Python view_succeeded:
// nil group records a front page view, nil repo a group view.
func (n *Node) viewSucceeded(group, repo string, remote *identity.Identity) {
	if n.statsIgnored(remote) {
		return
	}
	if n.cfg.RecordStats && n.stats != nil {
		n.stats.record(group, repo, statView)
	}
}

func (n *Node) fetchSucceeded(group, repo string, remote *identity.Identity) {
	if n.statsIgnored(remote) {
		return
	}
	if n.cfg.RecordStats && n.stats != nil && group != "" && repo != "" {
		n.stats.record(group, repo, statFetch)
	}
}

// pushSucceeded honors the push ignore list, which the reference config
// parses but never applies.
func (n *Node) pushSucceeded(group, repo string, remote *identity.Identity) {
	if n.statsPushIgnored(remote) {
		return
	}
	if n.cfg.RecordStats && n.stats != nil && group != "" && repo != "" {
		n.stats.record(group, repo, statPush)
	}
}

func (n *Node) downloadSucceeded(group, repo string, remote *identity.Identity) {
	if n.statsIgnored(remote) {
		return
	}
	if n.cfg.RecordStats && n.stats != nil && group != "" && repo != "" {
		n.stats.record(group, repo, statDownload)
	}
}

func (n *Node) releaseDownloadSucceeded(group, repo string, remote *identity.Identity) {
	if n.statsIgnored(remote) {
		return
	}
	if n.cfg.RecordStats && n.stats != nil && group != "" && repo != "" {
		n.stats.record(group, repo, statReleaseDownload)
	}
}

// repositoryStats aggregates the lookback window for the stats page. The
// scoring model matches the reference: views and downloads are weighted low,
// fetches and pushes dominate, and the level thresholds are inactive / low
// below 3 / moderate below 10 / high.
func (n *Node) repositoryStats(group, repo string, lookbackDays int) *repoStats {
	if lookbackDays <= 0 {
		lookbackDays = statsLookbackDays
	}
	now := time.Now()
	days := make([]string, lookbackDays)
	labels := make([]string, lookbackDays)
	for i := lookbackDays - 1; i >= 0; i-- {
		d := now.AddDate(0, 0, -i)
		days[lookbackDays-1-i] = d.Format("2006-01-02")
		labels[lookbackDays-1-i] = d.Format("Jan 02")
	}
	rs := &repoStats{
		Days:         days,
		DayLabels:    labels,
		LookbackDays: lookbackDays,
		Series:       map[string]*statSeries{},
	}
	for _, kind := range statKinds {
		rs.Series[kind] = &statSeries{Daily: make([]int64, lookbackDays)}
	}
	rs.DownloadsTotal = &statSeries{Daily: make([]int64, lookbackDays)}

	s := n.stats
	if s == nil {
		return rs
	}
	s.mu.Lock()
	repoData := s.repos[group][repo]
	s.mu.Unlock()

	earliest := ""
	for i, day := range days {
		for _, kind := range statKinds {
			c := repoData[kind][day]
			ser := rs.Series[kind]
			ser.Daily[i] = c
			ser.Total += c
			if c > ser.Peak {
				ser.Peak = c
				ser.PeakDay = day
			}
			if c > 0 && (earliest == "" || day < earliest) {
				earliest = day
			}
			if kind == statDownload || kind == statReleaseDownload {
				dl := rs.DownloadsTotal
				dl.Daily[i] += c
				dl.Total += c
				if dl.Daily[i] > dl.Peak {
					dl.Peak = dl.Daily[i]
					dl.PeakDay = day
				}
			}
		}
	}
	rs.DateRange = labels[0] + " - " + labels[len(labels)-1]

	viewTotal := rs.Series[statView].Total + rs.Series[statDownload].Total + rs.Series[statReleaseDownload].Total
	score := float64(viewTotal)*0.2 + float64(rs.Series[statFetch].Total)*2.0 + float64(rs.Series[statPush].Total)*5.0
	rs.ActivityScore = int(score)

	actual := lookbackDays
	if earliest != "" {
		if t, err := time.ParseInLocation("2006-01-02", earliest, time.Local); err == nil {
			span := int(math.Floor(now.Sub(t).Hours()/24)) + 1
			if span >= 1 && span < actual {
				actual = span
			}
		}
	}
	rs.ActualDays = actual
	daily := 0.0
	if actual > 0 {
		daily = score / float64(actual)
	}
	rs.ActivityLevel = activityLevel(daily)
	return rs
}

// activityLevel maps a per-day activity score to a label, matching the
// thresholds in the reference stats page.
func activityLevel(daily float64) string {
	switch {
	case daily == 0:
		return "inactive"
	case daily < 3:
		return "low"
	case daily < 10:
		return "moderate"
	default:
		return "high"
	}
}
