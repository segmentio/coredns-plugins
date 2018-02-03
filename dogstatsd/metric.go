package dogstatsd

import (
	"strconv"

	dto "github.com/prometheus/client_model/go"
)

type key struct {
	kind   kind
	name   string
	tags   tags
	bucket int
}

type kind int

const (
	counter   kind = 'c'
	gauge     kind = 'g'
	histogram kind = 'h'
)

type metric struct {
	kind  kind
	name  string
	value float64
	rate  float64
	tags  tags

	// bucket is the index of the histogram bucket that the metric was generated
	// from. The count, min, and max values are used to represent the number of
	// values observed by the histogram bucket and the range in which they were
	// observed.
	bucket int
	count  uint64
	min    float64
	max    float64
}

func makeMetrics(f *dto.MetricFamily, m *dto.Metric) []metric {
	tags := makeTags(m)

	switch *f.Type {
	case dto.MetricType_COUNTER:
		return []metric{{
			kind:  counter,
			name:  *f.Name,
			value: *m.Counter.Value,
			tags:  tags,
		}}

	case dto.MetricType_GAUGE:
		return []metric{{
			kind:  gauge,
			name:  *f.Name,
			value: *m.Gauge.Value,
			tags:  tags,
		}}

	case dto.MetricType_HISTOGRAM:
		buckets := m.Histogram.Bucket
		metrics := make([]metric, 0, len(buckets))
		name := *f.Name
		count := uint64(0)
		min := 0.0

		for index, bucket := range buckets {
			cum := *bucket.CumulativeCount
			max := *bucket.UpperBound

			metrics = append(metrics, metric{
				kind:   histogram,
				name:   name,
				tags:   tags,
				bucket: index,
				count:  cum - count,
				min:    min,
				max:    max,
			})

			count, min = cum, max
		}

		return metrics

	default:
		// case dto.MetricType_SUMMARY:
		// case dto.MetricType_UNTYPED:
		//
		// For now summary and untyped metrics are not used in coredns, so we
		// will skip generating them.
		return nil
	}
}

func appendMetric(b []byte, m metric) []byte {
	b = appendName(b, m.name)
	b = append(b, ':')
	b = strconv.AppendFloat(b, m.value, 'g', -1, 64)
	b = append(b, '|')
	b = append(b, byte(m.kind))

	if m.rate != 0 && m.rate != 1 {
		b = append(b, '|', '@')
		b = strconv.AppendFloat(b, m.rate, 'g', -1, 64)
	}

	if len(m.tags) != 0 {
		b = append(b, '|', '#')
		b = append(b, m.tags...)
	}

	return append(b, '\n')
}

func appendName(b []byte, s string) []byte {
	// Dogstatsd metric names must start with a letter. Here we are not checking
	// for this condition because in the context of coredns all metric names are
	// prefixed with "coredns_" and therefore state with a letter.
	for _, c := range s {
		switch {
		case isUnderscore(c):
			// Dogstatsd systems use periods as namespace delimiers, whereas
			// prometheus uses underscores. This may have the side effect of
			// replacing underscores with periods in the metric name part of
			// the prometheus metric, but it doesn't seem out of place in the
			// context of a dogstatsd metric. We could have tried to identify
			// the namespace and subsystem and only put periods for delimiters
			// between those, but this is error prone and hard to reason about
			// when it doesn't work as expected (think about a subsystem with
			// underscores). To keep things simple, we just replace all initial
			// underscores with periods.
			b = append(b, '.')
		case isValidNameRune(c):
			b = append(b, byte(c))
		default:
			b = append(b, '_')
		}
	}
	return b
}

func appendTagName(b []byte, s string) []byte {
	for _, c := range s {
		switch {
		case isColumn(c):
			// Not sure prometheus allows it but just in case.
			b = append(b, '_')
		case isValidTagRune(c):
			if isAlphaUpper(c) {
				c = toLower(c)
			}
			b = append(b, byte(c))
		default:
			b = append(b, '_')
		}
	}
	return b
}

func appendTagValue(b []byte, s string) []byte {
	for _, c := range s {
		switch {
		case isValidNameRune(c):
			if isAlphaUpper(c) {
				c = toLower(c)
			}
			b = append(b, byte(c))
		default:
			b = append(b, '_')
		}
	}
	return b
}

func isValidNameRune(c rune) bool {
	return isAlphaNum(c) || isUnderscore(c) || isPeriod(c)
}

func isValidTagRune(c rune) bool {
	return isAlphaNum(c) || isUnderscore(c) || isPeriod(c) || isMinus(c) || isSlash(c)
}

func isAlphaNum(c rune) bool   { return isAlpha(c) || isNum(c) }
func isAlpha(c rune) bool      { return isAlphaUpper(c) || isAlphaLower(c) }
func isAlphaUpper(c rune) bool { return c >= 'A' && c <= 'Z' }
func isAlphaLower(c rune) bool { return c >= 'a' && c <= 'z' }
func isNum(c rune) bool        { return c >= '0' && c <= '9' }
func isUnderscore(c rune) bool { return c == '_' }
func isPeriod(c rune) bool     { return c == '.' }
func isMinus(c rune) bool      { return c == '-' }
func isSlash(c rune) bool      { return c == '/' }
func isColumn(c rune) bool     { return c == ':' }
func toLower(c rune) rune      { return c + ('a' - 'A') }

type tags string

func makeTags(m *dto.Metric) tags {
	if len(m.Label) == 0 {
		return ""
	}

	b := make([]byte, 0, 20*len(m.Label))

	for i, p := range m.Label {
		if i != 0 {
			b = append(b, ',')
		}
		b = appendTagName(b, *p.Name)
		b = append(b, ':')
		b = appendTagValue(b, *p.Value)
	}

	return tags(b)
}

type state map[key]metric

func (s state) observe(m metric) (metrics []metric) {
	k := key{
		kind:   m.kind,
		name:   m.name,
		tags:   m.tags,
		bucket: m.bucket,
	}

	v, exist := s[k]

	switch m.kind {
	case counter:
		// For counters we report the difference from the last value seen for
		// the metric.
		if !exist {
			v = m
			metrics = append(metrics, m)
		} else {
			delta := m.value - v.value
			if delta < 0 { // counter reset
				delta = m.value
			}
			if delta != 0 {
				v.value = m.value
				m.value = delta
				metrics = append(metrics, m)
			}
		}

	case gauge:
		// For gauges we simply set the value, the prometheus and dogstatsd
		// metric models use the same gauge concept.
		if !exist {
			v = m
			metrics = append(metrics, m)
		} else if v.value != m.value {
			v.value = m.value
			metrics = append(metrics, m)
		}

	case histogram:
		// For histograms we generate a list of metrics that represent discrete
		// observations spread out across the histogram bucket range.
		// The difference in cumulative count is used to determine how many
		// metrics we generate.
		count := m.count - v.count
		if count < 0 { // histogram reset
			count = m.count
		}
		for i := uint64(0); i < count; i++ {
			h := m
			a := h.max - h.min
			b := float64(i) + 0.5
			c := float64(count)
			h.value = h.min + ((a * b) / c)
			metrics = append(metrics, h)
		}
		v = m
	}

	s[k] = v
	return
}
