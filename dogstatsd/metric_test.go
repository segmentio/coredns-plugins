package dogstatsd

import (
	"testing"
)

var testMetrics = []struct {
	s string
	m metric
}{
	{
		s: "test.metric.small:0|c\n",
		m: metric{
			kind:  counter,
			name:  "test.metric.small",
			value: 0,
			rate:  1,
		},
	},

	{
		s: "test.metric.common:1|c|#hello:world,answer:42\n",
		m: metric{
			kind:  counter,
			name:  "test.metric.common",
			value: 1,
			rate:  1,
			tags:  "hello:world,answer:42",
		},
	},

	{
		s: "test.metric.large:1.234|c|@0.1|#hello:world,hello:world,hello:world,hello:world,hello:world,hello:world,hello:world,hello:world,hello:world,hello:world\n",
		m: metric{
			kind:  counter,
			name:  "test.metric.large",
			value: 1.234,
			rate:  0.1,
			tags:  "hello:world,hello:world,hello:world,hello:world,hello:world,hello:world,hello:world,hello:world,hello:world,hello:world",
		},
	},

	{
		s: "page.views:1|c\n",
		m: metric{
			kind:  counter,
			name:  "page.views",
			value: 1,
			rate:  1,
		},
	},

	{
		s: "fuel.level:0.5|g\n",
		m: metric{
			kind:  gauge,
			name:  "fuel.level",
			value: 0.5,
			rate:  1,
		},
	},

	{
		s: "song.length:240|h|@0.5\n",
		m: metric{
			kind:  histogram,
			name:  "song.length",
			value: 240,
			rate:  0.5,
		},
	},

	{
		s: "users.uniques:1234|h\n",
		m: metric{
			kind:  histogram,
			name:  "users.uniques",
			value: 1234,
			rate:  1,
		},
	},

	{
		s: "users.online:1|c|#country:china\n",
		m: metric{
			kind:  counter,
			name:  "users.online",
			value: 1,
			rate:  1,
			tags:  "country:china",
		},
	},

	{
		s: "users.online:1|c|@0.5|#country:china\n",
		m: metric{
			kind:  counter,
			name:  "users.online",
			value: 1,
			rate:  0.5,
			tags:  "country:china",
		},
	},
}

func TestAppendMetric(t *testing.T) {
	for _, test := range testMetrics {
		t.Run(test.m.name, func(b *testing.T) {
			if s := string(appendMetric(nil, test.m)); s != test.s {
				t.Errorf("\n<<< %#v\n>>> %#v", test.s, s)
			}
		})
	}
}

func BenchmarkAppendMetric(b *testing.B) {
	buffer := make([]byte, 4096)

	for _, test := range testMetrics {
		b.Run(test.m.name, func(b *testing.B) {
			for i := 0; i != b.N; i++ {
				appendMetric(buffer[:0], test.m)
			}
		})
	}
}
