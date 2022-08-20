package main

import (
	"context"
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"
)

type proberMetrics struct {
	dnsResolveRequestDuration   *prometheus.HistogramVec
	dnsResolveRequestTotal      *prometheus.CounterVec
	dnsResolveRequestErrorTotal *prometheus.CounterVec
}

type prober struct {
	server   string
	client   *dns.Client
	interval time.Duration
	metrics  *proberMetrics
}

func NewProber(server string, timeout time.Duration, interval time.Duration, registry *prometheus.Registry) *prober {
	if _, port, _ := net.SplitHostPort(server); len(port) == 0 {
		server = net.JoinHostPort(server, "53")
	}

	client := &dns.Client{
		DialTimeout:  timeout,
		ReadTimeout:  timeout,
		WriteTimeout: timeout,
	}

	dnsResolveRequestDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        "dns_resolve_request_duration",
		Help:        "Duration of dns request by rcode and probe target",
		ConstLabels: map[string]string{"server": server},
	}, []string{"rcode", "target"})

	dnsResolveRequestTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name:        "dns_resolve_request_total",
		Help:        "Count of dns requests",
		ConstLabels: map[string]string{"server": server},
	}, []string{"target"})

	dnsResolveRequestErrorTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name:        "dns_resolve_request_error_total",
		Help:        "Count of dns request errors",
		ConstLabels: map[string]string{"server": server},
	}, []string{"target"})

	if registry != nil {
		registry.MustRegister(dnsResolveRequestDuration)
		registry.MustRegister(dnsResolveRequestTotal)
		registry.MustRegister(dnsResolveRequestErrorTotal)
	}

	return &prober{
		server:   server,
		client:   client,
		interval: interval,
		metrics: &proberMetrics{
			dnsResolveRequestTotal:      dnsResolveRequestTotal,
			dnsResolveRequestDuration:   dnsResolveRequestDuration,
			dnsResolveRequestErrorTotal: dnsResolveRequestErrorTotal,
		},
	}
}

func (p *prober) Start(ctx context.Context, address string) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
		rcode, rtt, err := p.probeOnce(address)
		if err != nil {
			Logger().Errorw("probe failed", "target", address, "error", err)
			continue
		}
		Logger().Infow("probed", "rcode", rcode, "rtt", rtt, "target", address)
	}
}

func (p *prober) probeOnce(address string) (string, time.Duration, error) {
	if !strings.HasSuffix(address, ".") {
		address = address + "."
	}

	msg := new(dns.Msg)
	msg.RecursionDesired = true
	msg.Question = make([]dns.Question, 0)
	msg.Question = append(msg.Question, dns.Question{
		Name:   address,
		Qtype:  dns.TypeA,
		Qclass: dns.ClassINET,
	})
	p.metrics.dnsResolveRequestTotal.With(prometheus.Labels{"target": address}).Inc()

	msg, rtt, err := p.client.Exchange(msg, p.server)
	if err != nil {
		p.metrics.dnsResolveRequestErrorTotal.With(prometheus.Labels{"target": address}).Inc()
		return "", -1, err
	}

	rcode := dns.RcodeToString[msg.Rcode]
	p.metrics.dnsResolveRequestDuration.With(prometheus.Labels{"rcode": rcode, "target": address}).Observe(rtt.Seconds())
	return rcode, rtt, err
}
