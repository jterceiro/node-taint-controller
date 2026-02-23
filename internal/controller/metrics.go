package controller

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// nodesTaintedTotal counts the total number of times the out-of-service taint has been applied.
	nodesTaintedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "nodes_tainted_total",
		Help: "Total number of times the out-of-service taint has been applied to nodes.",
	})

	// currentUnhealthyNodes tracks the current number of NotReady nodes being watched.
	currentUnhealthyNodes = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "current_unhealthy_nodes",
		Help: "Current number of nodes in a NotReady state.",
	})
)

func init() {
	metrics.Registry.MustRegister(nodesTaintedTotal, currentUnhealthyNodes)
}
