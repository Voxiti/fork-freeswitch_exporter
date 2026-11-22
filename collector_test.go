package main

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"testing"

	kitlog "github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
)

// collectorWithReply returns a collector whose event socket answers every
// command with body.
func collectorWithReply(t *testing.T, body string) *Collector {
	t.Helper()
	client, server := net.Pipe()
	t.Cleanup(func() { client.Close(); server.Close() })

	go func() {
		commands := bufio.NewReader(server)
		for {
			if _, err := commands.ReadString('\n'); err != nil {
				return
			}
			if _, err := commands.ReadString('\n'); err != nil {
				return
			}
			fmt.Fprintf(server, "Content-Type: api/response\nContent-Length: %d\n\n%s", len(body), body)
		}
	}()

	return &Collector{conn: client, input: bufio.NewReader(client), logger: kitlog.NewNopLogger()}
}

func TestIsCommandNotFound(t *testing.T) {
	for response, want := range map[string]bool{
		"-ERR conference Command not found!\n": true,
		"-ERR verto Command not found!\n":      true,
		"-ERR no reply\n":                      false,
		"0\n":                                  false,
		"":                                     false,
	} {
		if got := isCommandNotFound([]byte(response)); got != want {
			t.Errorf("isCommandNotFound(%q) = %v, want %v", response, got, want)
		}
	}
}

func TestFetchMetricReportsMissingCommand(t *testing.T) {
	c := collectorWithReply(t, "-ERR conference Command not found!\n")
	metric := Metric{Name: "current_conferences", Command: "api conference count"}

	if _, err := c.fetchMetric(&metric); !errors.Is(err, errCommandNotFound) {
		t.Fatalf("fetchMetric error = %v, want errCommandNotFound", err)
	}
}

func TestFetchMetricStillParsesValues(t *testing.T) {
	c := collectorWithReply(t, "3")
	metric := Metric{Name: "current_conferences", Command: "api conference count"}

	value, err := c.fetchMetric(&metric)
	if err != nil || value != 3 {
		t.Fatalf("fetchMetric = %v, %v, want 3, nil", value, err)
	}
}

func TestVertoMetricsSkipsMissingModule(t *testing.T) {
	c := collectorWithReply(t, "-ERR verto Command not found!\n")
	ch := make(chan prometheus.Metric, 1)

	if err := c.vertoMetrics(ch); err != nil {
		t.Fatalf("vertoMetrics error = %v, want nil", err)
	}
	if len(ch) != 0 {
		t.Fatalf("vertoMetrics sent %d metrics, want 0", len(ch))
	}
}
