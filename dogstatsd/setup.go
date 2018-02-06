package dogstatsd

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metrics"
	"github.com/mholt/caddy"
)

func init() {
	caddy.RegisterPlugin("dogstatsd", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

func setup(c *caddy.Controller) error {
	d, err := dogstatsdParse(c)
	if err != nil {
		return plugin.Error("dogstatsd", err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		d.Next = next
		return d
	})

	c.OnStartup(func() error {
		m, _ := dnsserver.GetConfig(c).Handler("prometheus").(*metrics.Metrics)
		if m == nil {
			return errors.New("the dogstatsd plugin requires the prometheus plugin to be loaded, add 'prometheus' to the zone configuration block where 'dogstatsd' is declared")
		}

		d.Reg = m.Reg
		d.ZoneNames = m.ZoneNames()
		d.Start()
		return nil
	})

	c.OnShutdown(func() error {
		d.Stop()
		return nil
	})
	return nil
}

func dogstatsdParse(c *caddy.Controller) (*Dogstatsd, error) {
	if !c.Next() { // 'dogstatsd'
		return nil, c.Err("expected 'dogstatsd' token at the beginning of the dogstatsd plugin configuration block")
	}

	d := New()

	switch args := c.RemainingArgs(); len(args) {
	case 0:
	case 1:
		d.Addr = args[0]
		if i := strings.Index(d.Addr, "://"); i < 0 {
			d.Addr = "udp://" + d.Addr
		} else {
			switch d.Addr[:i] {
			case "udp", "udp4", "udp6", "unixgram":
			default:
				return nil, c.Errf("unsupported protocol: %s", d.Addr[:i])
			}
		}
	default:
		return nil, c.ArgErr()
	}

	for c.NextBlock() {
		switch c.Val() {
		case "buffer":
			bufferSize, err := dogstatsdParseBuffer(c)
			if err != nil {
				return nil, err
			}
			d.BufferSize = bufferSize

		case "flush":
			flushInterval, err := dogstatsdParseFlush(c)
			if err != nil {
				return nil, err
			}
			d.FlushInterval = flushInterval

		case "go":
			if len(c.RemainingArgs()) != 0 {
				return nil, c.ArgErr()
			}
			d.EnableGoMetrics = true

		case "process":
			if len(c.RemainingArgs()) != 0 {
				return nil, c.ArgErr()
			}
			d.EnableProcessMetrics = true

		default:
			return nil, c.ArgErr()
		}
	}

	return d, nil
}

func dogstatsdParseBuffer(c *caddy.Controller) (bufferSize int, err error) {
	args := c.RemainingArgs()

	if len(args) != 1 {
		err = c.ArgErr()
		return
	}

	if bufferSize, err = strconv.Atoi(args[0]); err != nil {
		return
	}

	if bufferSize <= 512 {
		err = c.Errf("the buffer size must be at least 512 B, got %d B", bufferSize)
	}

	if bufferSize > 65536 {
		err = c.Errf("the buffer size must be at most 65536 B, got %d B", bufferSize)
	}

	return
}

func dogstatsdParseFlush(c *caddy.Controller) (flushInterval time.Duration, err error) {
	args := c.RemainingArgs()

	if len(args) != 1 {
		err = c.ArgErr()
		return
	}

	if flushInterval, err = time.ParseDuration(args[0]); err != nil {
		return
	}

	if flushInterval < (1 * time.Second) {
		err = c.Errf("the flush interval must be at least 1s, got %s", flushInterval)
	}

	return
}
