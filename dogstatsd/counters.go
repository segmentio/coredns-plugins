package dogstatsd

import (
	"sort"
	"sync"
)

// counterStore is a data type that keep tracks of counters indexed by keys and is
// used to retrieve the top N most popular keys.
type counterStore struct {
	mutex sync.Mutex
	index map[string]int64
}

func makeCounterStore() counterStore {
	return counterStore{index: make(map[string]int64, 1000)}
}

func (c *counterStore) incr(name string) {
	c.mutex.Lock()
	c.index[name]++
	c.mutex.Unlock()
}

func (c *counterStore) top(n int) []counterEntry {
	index := c.swap(make(map[string]int64, 1000))
	count := make([]counterEntry, 0, len(index))

	for key, value := range index {
		count = append(count, counterEntry{key: key, value: value})
	}

	sort.Sort(sort.Reverse(
		counterEntriesByValue(count),
	))

	if n < len(count) {
		count = count[:n]
	}
	return count
}

func (c *counterStore) swap(m map[string]int64) map[string]int64 {
	c.mutex.Lock()
	m, c.index = c.index, m
	c.mutex.Unlock()
	return m
}

type counterEntry struct {
	key   string
	value int64
}

func (c counterEntry) metric(name string, tag string) metric {
	return metric{
		kind:  counter,
		name:  name,
		value: float64(c.value),
		tags:  tags(tag + ":" + c.key),
	}
}

type counterEntriesByValue []counterEntry

func (c counterEntriesByValue) Len() int           { return len(c) }
func (c counterEntriesByValue) Less(i, j int) bool { return c[i].value < c[j].value }
func (c counterEntriesByValue) Swap(i, j int)      { c[i], c[j] = c[j], c[i] }
