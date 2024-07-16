package main

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"

	v1 "k8s.io/api/core/v1"
	scheduler "k8s.io/kubernetes/cmd/kube-scheduler/app"
	framework "k8s.io/kubernetes/pkg/scheduler/framework"
)

const (
	// PluginName is the name of the plugin used in the scheduler registry and configurations.
	PluginName = "NodeSelectorPlugin"
	nodeName   = "dev-cluster-worker2"
	promURL    = "http://prometheus-k8s.monitoring.svc:9090"
	promQuery  = `sum by (instance)(increase(kepler_container_joules_total{}[1m]))`
)

// NodeSelectorPlugin is a plugin that selects a specific node.
type NodeSelectorPlugin struct {
	handle     framework.Handle
	promClient promv1.API
}

var _ framework.FilterPlugin = &NodeSelectorPlugin{}

// Name returns the name of the plugin.
func (pl *NodeSelectorPlugin) Name() string {
	return PluginName
}

// Filter is the function that filters nodes based on the plugin logic.
//
//	func (pl *NodeSelectorPlugin) Filter(ctx context.Context, _ *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
//		if nodeInfo.Node().Name == nodeName {
//			return framework.NewStatus(framework.Success, fmt.Sprintf("Node %s is selected", nodeName))
//		}
//		return framework.NewStatus(framework.Unschedulable, fmt.Sprintf("Node %s is not selected", nodeInfo.Node().Name))
//	}
func (pl *NodeSelectorPlugin) Filter(ctx context.Context, _ *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	klog.Infof("Filtering node: %s for pod: %s/%s", nodeInfo.Node().Name, pod.Namespace, pod.Name)

	// Fetch Prometheus metrics
	result, err := pl.fetchPrometheusMetrics(ctx)
	if err != nil {
		klog.Errorf("Error fetching metrics: %v", err)
		return framework.NewStatus(framework.Error, fmt.Sprintf("Error fetching metrics: %v", err))
	}

	klog.Infof("Prometheus query result: %v", result)
	// Check if the node is in the metrics result
	for _, sample := range result {
		if string(sample.Metric["instance"]) == nodeInfo.Node().Name {
			klog.Infof("Node %s is selected based on Prometheus metrics", nodeInfo.Node().Name)
			return framework.NewStatus(framework.Success, fmt.Sprintf("Node %s is selected", nodeName))
		}
	}
	klog.Infof("Node %s is not selected based on Prometheus metrics", nodeInfo.Node().Name)
	return framework.NewStatus(framework.Unschedulable, fmt.Sprintf("Node %s is not selected", nodeInfo.Node().Name))
}

// fetchPrometheusMetrics queries Prometheus for the specified metrics.
func (pl *NodeSelectorPlugin) fetchPrometheusMetrics(ctx context.Context) (model.Vector, error) {
	klog.Infof("Querying Prometheus with query: %s", promQuery)
	result, warnings, err := pl.promClient.Query(ctx, promQuery, time.Now())
	if err != nil {
		return nil, err
	}
	if len(warnings) > 0 {
		klog.Infof("Warnings: %v", warnings)
	}
	klog.Infof("Prometheus query result: %v", result)
	return result.(model.Vector), nil
}

// // New initializes a new plugin and returns it.
// func New(plugin rt.Object, handle framework.Handle) (framework.Plugin, error) {
// 	return &NodeSelectorPlugin{handle: handle}, nil
// }

// New initializes a new plugin and returns it.
func New(_ runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	klog.Infof("Initializing %s plugin", PluginName)
	client, err := api.NewClient(api.Config{
		Address: promURL,
	})
	if err != nil {
		klog.Errorf("Error creating Prometheus client: %v", err)
		return nil, err
	}
	return &NodeSelectorPlugin{
		handle:     handle,
		promClient: promv1.NewAPI(client),
	}, nil
}

func main() {
	klog.InitFlags(nil)
	command := scheduler.NewSchedulerCommand(
		scheduler.WithPlugin(PluginName, New),
	)
	if err := command.Execute(); err != nil {
		klog.Fatal(err)
	}
}
