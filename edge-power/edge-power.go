package edgepower

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"k8s.io/klog/v2"
)

const (
    // raspberryPiPromURL   = "http://localhost:9090"
    raspberryPiPromURL   = "http://prometheus-k8s.monitoring.svc:9090"
    raspberryPiQuery     = `sum by (instance)(raspberry_pi_power_watts)`
)

// RaspberryPiPrometheusClient handles Prometheus queries for Raspberry Pi power metrics
type RaspberryPiPrometheusClient struct {
    client promv1.API
}

// NewRaspberryPiPrometheusClient initializes and returns a RaspberryPiPrometheusClient
func NewRaspberryPiPrometheusClient() (*RaspberryPiPrometheusClient, error) {
    client, err := api.NewClient(api.Config{
        Address: raspberryPiPromURL,
    })
    if err != nil {
        klog.Errorf("Error creating Prometheus client: %v", err)
        return nil, err
    }
    return &RaspberryPiPrometheusClient{
        client: promv1.NewAPI(client),
    }, nil
}

// GetAllRaspberryPiNodeEnergies queries Prometheus and returns energy consumption for all nodes
func (rpc *RaspberryPiPrometheusClient) GetAllRaspberryPiNodeEnergies() (map[string]float64, error) {
    klog.Infof("Querying Prometheus with query: %s", raspberryPiQuery)
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    result, warnings, err := rpc.client.Query(ctx, raspberryPiQuery, time.Now())
    if err != nil {
        klog.Errorf("Error querying Prometheus: %v", err)
        return nil, err
    }
    if len(warnings) > 0 {
        klog.Infof("Prometheus query warnings: %v", warnings)
    }

    klog.Infof("Prometheus query result: %v", result)
    return parseAllRaspberryPiNodeEnergies(result), nil
}

// parseAllRaspberryPiNodeEnergies extracts energy consumption for all nodes from the Prometheus query result.
func parseAllRaspberryPiNodeEnergies(result model.Value) map[string]float64 {
    energyMetrics := make(map[string]float64)
    vector, ok := result.(model.Vector)
    if !ok {
        klog.Errorf("Unexpected result format from Prometheus: %v", result)
        return energyMetrics
    }
    klog.Infof("parsing the result from prometheus")
    for _, sample := range vector {
        instance := string(sample.Metric["instance"])
        klog.Infof("found instance %s", instance)
        nodeIP := instance
        // Remove port from the instance string, assuming the format is "IP:PORT"
        if colonIndex := strings.LastIndex(instance, ":"); colonIndex != -1 {
            nodeIP = instance[:colonIndex]
        }
        value, err := strconv.ParseFloat(string(sample.Value.String()), 64)
        if err != nil {
            klog.Errorf("Error parsing energy value for node IP %s: %v", nodeIP, err)
            continue
        }
        energyMetrics[nodeIP] = value
    }

    return energyMetrics
}
