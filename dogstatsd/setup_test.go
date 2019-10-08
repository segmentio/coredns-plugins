package dogstatsd

import (
	"testing"
	"time"

	"github.com/caddyserver/caddy"
)

func TestSetupSuccess(t *testing.T) {
	tests := []struct {
		input                string
		addr                 string
		bufferSize           int
		flushInterval        time.Duration
		enableGoMetrics      bool
		enableProcessMetrics bool
	}{
		{
			input:         `dogstatsd`,
			addr:          defaultAddr,
			bufferSize:    defaultBufferSize,
			flushInterval: defaultFlushInterval,
		},

		{
			input:         `dogstatsd 10.50.0.2:8125`,
			addr:          "udp://10.50.0.2:8125",
			bufferSize:    defaultBufferSize,
			flushInterval: defaultFlushInterval,
		},

		{
			input:         `dogstatsd udp://10.50.0.2:8125`,
			addr:          "udp://10.50.0.2:8125",
			bufferSize:    defaultBufferSize,
			flushInterval: defaultFlushInterval,
		},

		{
			input: `dogstatsd {
				buffer 8192
			}`,
			addr:          defaultAddr,
			bufferSize:    8192,
			flushInterval: defaultFlushInterval,
		},

		{
			input: `dogstatsd {
				flush 10s
			}`,
			addr:          defaultAddr,
			bufferSize:    defaultBufferSize,
			flushInterval: 10 * time.Second,
		},

		{
			input: `dogstatsd {
				buffer 8192
				flush 10s
			}`,
			addr:          defaultAddr,
			bufferSize:    8192,
			flushInterval: 10 * time.Second,
		},

		{
			input: `dogstatsd {
				go
			}`,
			addr:            defaultAddr,
			bufferSize:      defaultBufferSize,
			flushInterval:   defaultFlushInterval,
			enableGoMetrics: true,
		},

		{
			input: `dogstatsd {
				process
			}`,
			addr:                 defaultAddr,
			bufferSize:           defaultBufferSize,
			flushInterval:        defaultFlushInterval,
			enableProcessMetrics: true,
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			t.Log(test.input)

			c := caddy.NewTestController("dns", test.input)
			d, err := dogstatsdParse(c)

			if err != nil {
				t.Errorf("Expected to parse successfully but got and error: %v", err)
				return
			}

			if d.Addr != test.addr {
				t.Errorf("Expected address to be %v but found: %v", test.addr, d.Addr)
			}

			if d.BufferSize != test.bufferSize {
				t.Errorf("Expected buffer size to be %v%% but found: %v%%", test.bufferSize, d.BufferSize)
			}

			if d.FlushInterval != test.flushInterval {
				t.Errorf("Expected flush interval to be %v but found: %v", test.flushInterval, d.FlushInterval)
			}

			if d.EnableGoMetrics != test.enableGoMetrics {
				t.Errorf("Expected go metrics to be %t but found: %t", test.enableGoMetrics, d.EnableGoMetrics)
			}

			if d.EnableProcessMetrics != test.enableProcessMetrics {
				t.Errorf("Expected process metrics to be %t but found: %t", test.enableProcessMetrics, d.EnableProcessMetrics)
			}
		})
	}
}

func TestSetupFailure(t *testing.T) {
	tests := []string{
		`dogstatsd http://localhost:8125 # unsupported address scheme`,
		`dogstatsd localhost 8125 # too may arguments`,
		`dogstatsd { # missing argument to 'buffer'
			buffer
		}`,
		`dogstatsd { # invalid argument to 'buffer'
			buffer whatever
		}`,
		`dogstatsd { # 'buffer' is too small
			buffer 511
		}`,
		`dogstatsd { # 'buffer' is too large
			buffer 65537
		}`,
		`dogstatsd { # too many arguments to 'buffer'
			buffer 1024 whatever
		}`,
		`dogstatsd { # missing argument to 'flush'
			flush
		}`,
		`dogstatsd { # invalid first argument to 'flush'
			flush whatever
		}`,
		`dogstatsd { # negative first arguemnt to 'flush'
			flush -1m
		}`,
		`dogstatsd { # too many arguments to 'flush'
			flush 1m% whatever
		}`,
		`dogstatsd { # invalid plugin configuration entry
			whatever
		}`,
		`dogstats { # too may arguments to 'go'
			go hello
		}`,
		`dogstats { # too may arguments to 'process'
			process hello
		}`,
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			t.Log(test)
			c := caddy.NewTestController("dns", test)
			_, err := dogstatsdParse(c)
			if err == nil {
				t.Error("Expected an error but found <nil>")
			}
		})
	}
}
