# consul

## Name
# dogstatsd

## Name

*dogstatsd* - publish coredns metrics to dogstatsd agents

## Description

CoreDNS has a builtin mechanism for exposing metrics to Prometheus over a HTTP
endpoint, but infrastructures that use a metric collection system based on
dogstatsd have to run a bridge to poll CoreDNS's metrics endpoint and push them
to a dogstatsd agent.

The *dogstatsd* plugin removes the need for a such bridge by allowing CoreDNS to
directly push its metrics to a dogstatsd agent over UDP.

## Syntax

~~~ txt
dogstatsd [ADDR:PORT]
~~~

* **ADDR** Address at which a dogstatsd agent is available. It may be prefixed
with udp://, udp4://, udp6://, or unixgram:// to indicate the protocal to use
to push metrics to a dogstatsd agent. If unxigram:// is specified the address
must be a path to a unix domain socket on the file system.
* **PORT** Port number at which the dogstatsd agent is accepting metrics. The
port must not be set when the unixgram:// protocol is used to push metrics to
a dogstatsd agent.

If you want more control:

~~~ txt
dogstatsd [ADDR:PORT] {
    buffer SIZE
    flush INTERVAL
}
~~~

* **buffer** configures the size of the client buffer used to push metrics to a
dogstatsd agent. This must not exceed the size of the receive buffer used by the
agent. The minimum size is 512 B, the maximum is 64 KB.
* **flush** configures the time interval between flushes of metrics to a
dogstatsd agent. The minimum interval is 1 second, there is not maximum.

## Examples

Enable the dogstatsd plugin with a client buffer size of 8 KB, and flushing
metrics every 10 seconds.

~~~ corefile
. {
    dogstatsd localhost:8125 {
        buffer 8192
        flush 10s
    }
}
~~~

### plugins.cfg

This plugin is intended to appear right after the prometheus plugin declaration.
