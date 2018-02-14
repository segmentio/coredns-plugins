package dogstatsd

import (
	"sort"
	"sync"
)

// nameCounters is a data type that keep tracks of counts by names and can be
// used to retrieve the top N most popular names.
type nameCounters struct {
	mutex    sync.Mutex
	counters map[string]int64
}

func makeNameCounters() nameCounters {
	return nameCounters{counters: make(map[string]int64, 1000)}
}

func (nc *nameCounters) incr(name string) {
	nc.mutex.Lock()
	nc.counters[name]++
	nc.mutex.Unlock()
}

func (nc *nameCounters) top(n int) []nameCounter {
	counters := nc.swap(make(map[string]int64, 1000))
	names := make([]nameCounter, 0, len(counters))

	for name, count := range counters {
		names = append(names, nameCounter{
			name:  name,
			count: count,
		})
	}

	sort.Sort(sort.Reverse(
		nameCountersByCount(names),
	))

	if n < len(names) {
		names = names[:n]
	}

	return names
}

func (nc *nameCounters) swap(counters map[string]int64) map[string]int64 {
	nc.mutex.Lock()
	counters, nc.counters = nc.counters, counters
	nc.mutex.Unlock()
	return counters
}

type nameCounter struct {
	name  string
	count int64
}

type nameCountersByCount []nameCounter

func (names nameCountersByCount) Len() int           { return len(names) }
func (names nameCountersByCount) Less(i, j int) bool { return names[i].count < names[j].count }
func (names nameCountersByCount) Swap(i, j int)      { names[i], names[j] = names[j], names[i] }
