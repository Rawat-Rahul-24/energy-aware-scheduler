package keplerpower

import (
    "context"
    "time"

    "github.com/prometheus/client_golang/api"
    promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
    "github.com/prometheus/common/model"
    "k8s.io/klog/v2"
)

const (
    promURL   = "http://prometheus-k8s.monitoring.svc:9090"
    promQuery = `sum by (instance)(increase(kepler_container_joules_total{}[1m]))`
)

// PrometheusClient handles Prometheus queries
type PrometheusClient struct {
    client promv1.API
}

// NewPrometheusClient initializes and returns a PrometheusClient
func NewPrometheusClient() (*PrometheusClient, error) {
    client, err := api.NewClient(api.Config{
        Address: promURL,
    })
    if err != nil {
        klog.Errorf("Error creating Prometheus client: %v", err)
        return nil, err
    }
    return &PrometheusClient{
        client: promv1.NewAPI(client),
    }, nil
}

// GetNodeEnergyConsumption queries Prometheus for energy metrics and returns the energy consumption for a specific node
func (pc *PrometheusClient) GetAllNodeEnergyConsumptions() (map[string]float64, error) {
    klog.Infof("Querying Prometheus with query: %s", promQuery)
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    result, warnings, err := pc.client.Query(ctx, promQuery, time.Now())
    if err != nil {
        klog.Errorf("Error querying Prometheus: %v", err)
        return nil, err
    }
    if len(warnings) > 0 {
        klog.Infof("Prometheus query warnings: %v", warnings)
    }

    klog.Infof("Prometheus query result: %v", result)
    return parseAllNodeEnergyConsumptions(result), nil
}

func parseAllNodeEnergyConsumptions(result model.Value) map[string]float64 {
    energyMetrics := make(map[string]float64)
    vector, ok := result.(model.Vector)
    if !ok {
        klog.Errorf("Unexpected result format from Prometheus: %v", result)
        return energyMetrics
    }

    for _, sample := range vector {
        nodeName := string(sample.Metric["instance"])
        energyMetrics[nodeName] = float64(sample.Value)
    }

    return energyMetrics
}