package dogstatsd

import (
	"reflect"
	"testing"
)

func TestCounterStore(t *testing.T) {
	c := makeCounterStore()

	for i := 0; i != 10; i++ {
		c.incr("www.segment.com.")
	}

	for i := 0; i != 4; i++ {
		c.incr("www.github.com.")
	}

	for i := 0; i != 3; i++ {
		c.incr("www.google.com.")
	}

	c.incr("google.com.")
	c.incr("facebook.com.")
	c.incr("datadoghq.com.")

	top3 := c.top(3)

	if !reflect.DeepEqual(top3, []counterEntry{
		{key: "www.segment.com.", value: 10},
		{key: "www.github.com.", value: 4},
		{key: "www.google.com.", value: 3},
	}) {
		t.Error("top counters mismatch:", top3)
	}
}
