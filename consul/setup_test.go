package coredns_consul

import (
	"testing"
	"time"

	"github.com/mholt/caddy"
)

func TestSetupSuccess(t *testing.T) {
	tests := []struct {
		input              string
		addr               string
		ttl                time.Duration
		prefetchAmount     int
		prefetchPercentage int
		prefetchDuration   time.Duration
	}{
		// valid inputs
		{
			input:              `consul`,
			addr:               defaultAddr,
			ttl:                defaultTTL,
			prefetchAmount:     defaultPrefetchAmount,
			prefetchPercentage: defaultPrefetchPercentage,
			prefetchDuration:   defaultPrefetchDuration,
		},

		{
			input:              `consul 1.2.3.4:1234`,
			addr:               "http://1.2.3.4:1234",
			ttl:                defaultTTL,
			prefetchAmount:     defaultPrefetchAmount,
			prefetchPercentage: defaultPrefetchPercentage,
			prefetchDuration:   defaultPrefetchDuration,
		},

		{
			input: `consul {
				ttl 10s
			}`,
			addr:               defaultAddr,
			ttl:                10 * time.Second,
			prefetchAmount:     defaultPrefetchAmount,
			prefetchPercentage: defaultPrefetchPercentage,
			prefetchDuration:   defaultPrefetchDuration,
		},

		{
			input: `consul {
				prefetch 12
			}`,
			addr:               defaultAddr,
			ttl:                defaultTTL,
			prefetchAmount:     12,
			prefetchPercentage: defaultPrefetchPercentage,
			prefetchDuration:   defaultPrefetchDuration,
		},

		{
			input: `consul {
				prefetch 12 30s
			}`,
			addr:               defaultAddr,
			ttl:                defaultTTL,
			prefetchAmount:     12,
			prefetchPercentage: defaultPrefetchPercentage,
			prefetchDuration:   30 * time.Second,
		},

		{
			input: `consul {
				prefetch 12 30s 50%
			}`,
			addr:               defaultAddr,
			ttl:                defaultTTL,
			prefetchAmount:     12,
			prefetchPercentage: 50,
			prefetchDuration:   30 * time.Second,
		},

		{
			input: `consul http://localhost:1234 {
				ttl 10s
				prefetch 12 30s 50%
			}`,
			addr:               "http://localhost:1234",
			ttl:                10 * time.Second,
			prefetchAmount:     12,
			prefetchPercentage: 50,
			prefetchDuration:   30 * time.Second,
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			t.Log(test.input)

			c := caddy.NewTestController("dns", test.input)
			consulPlugin, err := parseConsul(c)

			if err != nil {
				t.Errorf("Expected to parse successfully but got and error: %v", err)
				return
			}

			if consulPlugin.Addr != test.addr {
				t.Errorf("Expected consul address to be %v but found: %v", test.addr, consulPlugin.Addr)
			}

			if consulPlugin.TTL != test.ttl {
				t.Errorf("Expected TTL to be %v but found: %v", test.ttl, consulPlugin.TTL)
			}

			if consulPlugin.PrefetchAmount != test.prefetchAmount {
				t.Errorf("Expected prefetch amount to be %v but found: %v", test.prefetchAmount, consulPlugin.PrefetchAmount)
			}

			if consulPlugin.PrefetchPercentage != test.prefetchPercentage {
				t.Errorf("Expected prefetch percentage to be %v%% but found: %v%%", test.prefetchPercentage, consulPlugin.PrefetchPercentage)
			}

			if consulPlugin.PrefetchDuration != test.prefetchDuration {
				t.Errorf("Expectedprefetch duration to be %v but found: %v", test.prefetchDuration, consulPlugin.PrefetchDuration)
			}
		})
	}
}

func TestSetupFailure(t *testing.T) {
	tests := []string{
		`consul { # missing argument to 'ttl'
			ttl
		}`,
		`consul { # invalid argument to 'ttl'
			ttl whatever
		}`,
		`consul { # too many arguments to 'ttl'
			ttl 10s whatever
		}`,
		`consul { # missing argument to 'prefetch'
			prefetch
		}`,
		`consul { # invalid first argument to 'prefetch'
			prefetch whatever
		}`,
		`consul { # negative first arguemnt to 'prefetch'
			prefetch -1
		}`,
		`consul { # invalid second argument to 'prefetch'
			prefetch 10 whatever
		}`,
		`consul { # negative second argument to 'prefetch'
			prefetch 10 -1s
		}`,
		`consul { # invalid third argument to 'prefetch'
			prefetch 10 1s whatever
		}`,
		`consul { # invalid third argument to 'prefetch'
			prefetch 10 1s 1.5%
		}`,
		`consul { # negative third argument to 'prefetch'
			prefetch 10 1s -1%
		}`,
		`consul { # too many arguments to 'prefetch'
			prefetch 10 1s 10% whatever
		}`,
		`consul { # invalid plugin configuration entry
			whatever
		}`,
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			t.Log(test)
			c := caddy.NewTestController("dns", test)
			_, err := parseConsul(c)
			if err == nil {
				t.Error("Expected an error but found <nil>")
			}
		})
	}
}
