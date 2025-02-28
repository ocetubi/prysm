package powchain

import (
	"context"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prysmaticlabs/prysm/monitoring/clientstats"
)

type BeaconNodeStatsUpdater interface {
	Update(stats clientstats.BeaconNodeStats)
}

type PowchainCollector struct {
	SyncEth1Connected          *prometheus.Desc
	SyncEth1FallbackConnected  *prometheus.Desc
	SyncEth1FallbackConfigured *prometheus.Desc // true if flag specified: --fallback-web3provider
	updateChan                 chan clientstats.BeaconNodeStats
	latestStats                clientstats.BeaconNodeStats
	sync.Mutex
	ctx        context.Context
	finishChan chan struct{}
}

var _ BeaconNodeStatsUpdater = &PowchainCollector{}
var _ prometheus.Collector = &PowchainCollector{}

// Update satisfies the BeaconNodeStatsUpdater
func (pc *PowchainCollector) Update(update clientstats.BeaconNodeStats) {
	pc.updateChan <- update
}

// Describe is invoked by the prometheus collection loop.
// It returns a set of metric Descriptor references which
// are also used in Collect to group collected metrics into
// a family. Describe and Collect together satisfy the
// prometheus.Collector interface.
func (pc *PowchainCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- pc.SyncEth1Connected
	ch <- pc.SyncEth1FallbackConfigured
	ch <- pc.SyncEth1FallbackConnected
}

// Collect is invoked by the prometheus collection loop.
// It returns a set of Metrics representing the observation
// for the current collection period. In the case of this
// collector, we use values from the latest BeaconNodeStats
// value sent by the powchain Service, which updates this value
// whenever an internal event could change the state of one of
// the metrics.
// Describe and Collect together satisfy the
// prometheus.Collector interface.
func (pc *PowchainCollector) Collect(ch chan<- prometheus.Metric) {
	bs := pc.getLatestStats()

	var syncEth1FallbackConfigured float64 = 0
	if bs.SyncEth1FallbackConfigured {
		syncEth1FallbackConfigured = 1
	}
	ch <- prometheus.MustNewConstMetric(
		pc.SyncEth1FallbackConfigured,
		prometheus.GaugeValue,
		syncEth1FallbackConfigured,
	)

	var syncEth1FallbackConnected float64 = 0
	if bs.SyncEth1FallbackConnected {
		syncEth1FallbackConnected = 1
	}
	ch <- prometheus.MustNewConstMetric(
		pc.SyncEth1FallbackConnected,
		prometheus.GaugeValue,
		syncEth1FallbackConnected,
	)

	var syncEth1Connected float64 = 0
	if bs.SyncEth1Connected {
		syncEth1Connected = 1
	}
	ch <- prometheus.MustNewConstMetric(
		pc.SyncEth1Connected,
		prometheus.GaugeValue,
		syncEth1Connected,
	)
}

func (pc *PowchainCollector) getLatestStats() clientstats.BeaconNodeStats {
	pc.Lock()
	defer pc.Unlock()
	return pc.latestStats
}

func (pc *PowchainCollector) setLatestStats(bs clientstats.BeaconNodeStats) {
	pc.Lock()
	pc.latestStats = bs
	pc.Unlock()
}

// unregister returns true if the prometheus DefaultRegistry
// confirms that it was removed.
func (pc *PowchainCollector) unregister() bool {
	return prometheus.Unregister(pc)
}

func (pc *PowchainCollector) latestStatsUpdateLoop() {
	for {
		select {
		case <-pc.ctx.Done():
			pc.unregister()
			pc.finishChan <- struct{}{}
			return
		case bs := <-pc.updateChan:
			pc.setLatestStats(bs)
		}
	}
}

func NewPowchainCollector(ctx context.Context) (*PowchainCollector, error) {
	namespace := "powchain"
	updateChan := make(chan clientstats.BeaconNodeStats, 2)
	c := &PowchainCollector{
		SyncEth1FallbackConfigured: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "sync_eth1_fallback_configured"),
			"Boolean recording whether a fallback eth1 endpoint was configured: 0=false, 1=true.",
			nil,
			nil,
		),
		SyncEth1FallbackConnected: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "sync_eth1_fallback_connected"),
			"Boolean indicating whether a fallback eth1 endpoint is currently connected: 0=false, 1=true.",
			nil,
			nil,
		),
		SyncEth1Connected: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "sync_eth1_connected"),
			"Boolean indicating whether an eth1 endpoint is currently connected: 0=false, 1=true.",
			nil,
			nil,
		),
		updateChan: updateChan,
		ctx:        ctx,
		finishChan: make(chan struct{}, 1),
	}
	go c.latestStatsUpdateLoop()
	return c, prometheus.Register(c)
}

type NopBeaconNodeStatsUpdater struct{}

func (nop *NopBeaconNodeStatsUpdater) Update(_ clientstats.BeaconNodeStats) {}

var _ BeaconNodeStatsUpdater = &NopBeaconNodeStatsUpdater{}
