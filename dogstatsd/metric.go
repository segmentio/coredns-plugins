package dogstatsd

import (
	"strconv"
	"strings"

	"github.com/coredns/coredns/plugin"
	dto "github.com/prometheus/client_model/go"
)

type key struct {
	kind  kind
	name  string
	tags  tags
	index int
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

	// index of the histogram bucket that the metric was generated from.
	index   int
	count   uint64
	version uint64
}

func makeMetrics(f *dto.MetricFamily, m *dto.Metric, rand func(min, max float64) float64) []metric {
	name := makeName(*f.Name)
	tags := makeTags(m)

	switch *f.Type {
	case dto.MetricType_COUNTER:
		return []metric{{
			kind:  counter,
			name:  name,
			value: *m.Counter.Value,
			tags:  tags,
		}}

	case dto.MetricType_GAUGE:
		return []metric{{
			kind:  gauge,
			name:  name,
			value: *m.Gauge.Value,
			tags:  tags,
		}}

	case dto.MetricType_HISTOGRAM:
		buckets := m.Histogram.Bucket
		metrics := make([]metric, 0, len(buckets))
		acc := uint64(0)
		min := 0.0

		for index, bucket := range buckets {
			cct := *bucket.CumulativeCount
			max := *bucket.UpperBound

			metrics = append(metrics, metric{
				kind:    histogram,
				name:    name,
				value:   rand(min, max),
				tags:    tags,
				index:   index,
				count:   cct - acc,
				version: cct,
			})

			acc = cct
			min = max
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

func makeName(s string) string {
	const prefix = plugin.Namespace + "_"
	if !strings.HasPrefix(s, prefix) {
		return prefix + s
	}
	return s
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
		case isAlphaUpper(c):
			c = toLower(c)
			fallthrough
		case isValidTagRune(c):
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
		case isAlphaUpper(c):
			c = toLower(c)
			fallthrough
		case isValidTagRune(c):
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
	return isAlphaNum(c) || isUnderscore(c) || isPeriod(c) || isMinus(c) || isSlash(c) || isColumn(c)
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

func (s state) observe(m metric) (metric, bool) {
	k := key{
		kind:  m.kind,
		name:  m.name,
		tags:  m.tags,
		index: m.index,
	}

	v, ok := s[k]

	switch m.kind {
	case counter:
		// For counters we report the difference from the last value seen for
		// the metric.
		delta := m.value - v.value
		if delta < 0 { // counter reset
			delta = m.value
		}
		if ok = delta != 0; ok {
			v, m.value = m, delta
		}

	case gauge:
		// For gauges we simply set the value, the prometheus and dogstatsd
		// metric models use the same gauge concept.
		//
		// Gauges are reported on every flush so the metric collection system
		// does not "expire" them if it doesn't receive a value for a while.
		v, ok = m, true

	case histogram:
		// For histograms the appraoch is a bit more complex. Each bucket of a
		// prometheus histogram is reported as an individual metric where the
		// count is the number of changes that the bucket itself observed, and
		// the version is the cummulative count of changes on the bucket and the
		// buckets before it.
		//
		// Those two values must be taken into account to determine whether a
		// change occured on the metric. If the version is the same as the last
		// one we seen for the metric, then we are sure there were no changes.
		// However the version may have changed due to a chance in a sub-bucket,
		// which means we must also check the count of changes on the current
		// bucket to determine where the changes occured.
		//
		// Finally, we adjust the rate of the metric to represent the weight of
		// each bucket.
		count := m.count - v.count
		if ok = v.version != m.version && count != 0; ok {
			m.rate = 1 / float64(count)
			v = m
		}
	}

	s[k] = v
	return m, ok
}

const (
	goMetricPrefix      = plugin.Namespace + "_go_"
	processMetricPrefix = plugin.Namespace + "_process_"
)

func isGoMetric(name string) bool      { return strings.HasPrefix(name, goMetricPrefix) }
func isProcessMetric(name string) bool { return strings.HasPrefix(name, processMetricPrefix) }
