package consul

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/caddyserver/caddy"
)

func init() {
	caddy.RegisterPlugin("consul", caddy.Plugin{
		ServerType: "dns",
		Action:     setupConsul,
	})
}

// setupConsulPlugin configures the consul plugin, the format is:
//
//	consul [ADDR:PORT] {
//		ttl DURATION
//		prefetch AMOUNT [DURATION [PERCENTAGE%]]
//	}
//
func setupConsul(c *caddy.Controller) error {
	consulPlugin, err := parseConsul(c)
	if err != nil {
		return err
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		consulPlugin.Next = next
		return consulPlugin
	})

	c.OnStartup(func() error { return registerMetrics(c) })
	return nil
}

func parseConsul(c *caddy.Controller) (*Consul, error) {
	if !c.Next() { // 'consul'
		return nil, c.Err("expected 'consul' token at the beginning of the consul plugin configuration block")
	}

	consulPlugin := New()

	switch args := c.RemainingArgs(); len(args) {
	case 0:
	case 1:
		consulPlugin.Addr = args[0]
		if strings.Index(consulPlugin.Addr, "://") < 0 {
			consulPlugin.Addr = "http://" + consulPlugin.Addr
		}
	default:
		return nil, c.ArgErr()
	}

	for c.NextBlock() {
		switch c.Val() {
		case "prefetch":
			amount, percentage, duration, err := parsePrefetch(c)
			if err != nil {
				return nil, err
			}
			consulPlugin.PrefetchAmount = amount
			consulPlugin.PrefetchPercentage = percentage
			consulPlugin.PrefetchDuration = duration

		case "ttl":
			ttl, err := parseTTL(c)
			if err != nil {
				return nil, err
			}
			consulPlugin.TTL = ttl

		default:
			return nil, c.ArgErr()
		}
	}

	return consulPlugin, nil
}

func parsePrefetch(c *caddy.Controller) (amount int, percentage int, duration time.Duration, err error) {
	amount = defaultPrefetchAmount
	percentage = defaultPrefetchPercentage
	duration = defaultPrefetchDuration

	args := c.RemainingArgs()
	if len(args) == 0 || len(args) > 3 {
		err = c.ArgErr()
		return
	}

	if amount, err = strconv.Atoi(args[0]); err != nil {
		return
	}
	if amount <= 0 {
		err = fmt.Errorf("prefetch amount must be positive: %d", amount)
		return
	}

	if len(args) > 1 {
		if duration, err = time.ParseDuration(args[1]); err != nil {
			return
		}
		if duration <= 0 {
			err = fmt.Errorf("duration must be positive: %s", duration)
			return
		}
	}

	if len(args) > 2 {
		arg := args[2]
		if !strings.HasSuffix(arg, "%") {
			err = fmt.Errorf("last character of percentage must be `%%`, but is: %q", arg)
			return
		}
		arg = strings.TrimSuffix(arg, "%")
		if percentage, err = strconv.Atoi(arg); err != nil {
			return
		}
		if percentage < 10 || percentage > 90 {
			err = fmt.Errorf("percentage must fall in range [10, 90]: %d", percentage)
			return
		}
	}

	return
}

func parseTTL(c *caddy.Controller) (ttl time.Duration, err error) {
	args := c.RemainingArgs()

	if len(args) != 1 {
		err = c.ArgErr()
		return
	}

	if ttl, err = time.ParseDuration(args[0]); err != nil {
		return
	}

	if ttl < 0 {
		err = fmt.Errorf("ttl must be positive: %s", ttl)
	}

	return
}
