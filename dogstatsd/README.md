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
consul [ADDR:PORT]
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

* **ttl** configured how long responses from querying lists of services from
  consul are cached for. **DURATION** defaults to 1m.
* `prefetch` will prefetch popular items when they are about to be expunged
  from the cache.
  Popular means **AMOUNT** queries have been seen with no gaps of **DURATION**
  or more between them. **DURATION** defaults to 1m. Prefetching will happen
  when the TTL drops below **PERCENTAGE**, which defaults to `10%`, or latest 1
  second before TTL expiration. Values should be in the range `[10%, 90%]`.
  Note the percent sign is mandatory. **PERCENTAGE** is treated as an `int`.

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
