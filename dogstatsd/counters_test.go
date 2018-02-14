package dogstatsd

import (
	"reflect"
	"testing"
)

func TestNameCounters(t *testing.T) {
	nc := makeNameCounters()

	for i := 0; i != 10; i++ {
		nc.incr("www.segment.com.")
	}

	for i := 0; i != 4; i++ {
		nc.incr("www.github.com.")
	}

	for i := 0; i != 3; i++ {
		nc.incr("www.google.com.")
	}

	nc.incr("google.com.")
	nc.incr("facebook.com.")
	nc.incr("datadoghq.com.")

	top3 := nc.top(3)

	if !reflect.DeepEqual(top3, []nameCounter{
		{name: "www.segment.com.", count: 10},
		{name: "www.github.com.", count: 4},
		{name: "www.google.com.", count: 3},
	}) {
		t.Error("top counters mismatch:", top3)
	}
}
