package main

import (
	"context"
	keplerpower "custom-scheduler/kepler-power"
	edgepower "custom-scheduler/edge-power"
	"fmt"
	"os"


	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	scheduler "k8s.io/kubernetes/cmd/kube-scheduler/app"
	framework "k8s.io/kubernetes/pkg/scheduler/framework"
)

const (
    PluginName = "NodeSelectorPlugin"
    EnergyDataKey  = "EnergyDataKey"
)

type NodeSelectorPlugin struct {
    handle framework.Handle
    pc     *keplerpower.PrometheusClient
    rpc    *edgepower.RaspberryPiPrometheusClient
	nodeIPMap map[string]string 
}

var _ framework.FilterPlugin = &NodeSelectorPlugin{}


type EnergyDataState struct {
	MeanEnergy float64
	NodeEnergies map[string]float64
}

// Clone returns a copy of the EnergyMeanState, as required by the StateData interface
func (e *EnergyDataState) Clone() framework.StateData {
	return &EnergyDataState{
		MeanEnergy: e.MeanEnergy,
		NodeEnergies: e.NodeEnergies,
	}
}

func (pl *NodeSelectorPlugin) Name() string {
    return PluginName
}

// Filter function to fetch and calculate mean energy, then filter nodes accordingly
func (pl *NodeSelectorPlugin) Filter(ctx context.Context, cycleState *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	klog.Infof("Filtering node: %s for pod: %s/%s", nodeInfo.Node().Name, pod.Namespace, pod.Name)

	// Fetch energy data for all nodes once
	// Fetch energy data for all nodes once and store in CycleState
	stateData, err := cycleState.Read(EnergyDataKey)
	if err != nil {
		energyMetrics, err := pl.getAllNodeEnergies()
		if err != nil {
			return framework.NewStatus(framework.Error, fmt.Sprintf("Error getting node energies: %v", err))
		}

		var totalEnergy float64
		var nodeCount int
		nodeEnergies := make(map[string]float64)

		nodes, err := pl.handle.SnapshotSharedLister().NodeInfos().List()
		if err != nil {
			return framework.NewStatus(framework.Error, fmt.Sprintf("Error listing nodes: %v", err))
		}

		for _, node := range nodes {
			nodeName := node.Node().Name
			nodeIP := pl.nodeIPMap[nodeName]

			if nodeEnergy, exists := energyMetrics[nodeName]; exists {
				totalEnergy += nodeEnergy
				nodeEnergies[nodeName] = nodeEnergy
				nodeCount++
			} else if nodeEnergy, exists := energyMetrics[nodeIP]; exists {
				totalEnergy += nodeEnergy
				nodeEnergies[nodeName] = nodeEnergy
				nodeCount++
			}
		}

		if nodeCount == 0 {
			return framework.NewStatus(framework.Unschedulable, "No nodes available for scheduling")
		}
		klog.Infof("Total Energy: %f, Node Count: %d", totalEnergy, nodeCount)
		meanEnergy := totalEnergy / float64(nodeCount)
		klog.Infof("Mean Energy calculated: %f", meanEnergy)
		energyState := &EnergyDataState{MeanEnergy: meanEnergy, NodeEnergies: nodeEnergies}
		cycleState.Write(EnergyDataKey, energyState)
		stateData = energyState
	}

	energyState := stateData.(*EnergyDataState)
	klog.Infof("Mean Energy retrived from state: %f", energyState.MeanEnergy)
	// Check if the node's energy is available
	if nodeEnergy, exists := energyState.NodeEnergies[nodeInfo.Node().Name]; exists {
		if nodeEnergy > energyState.MeanEnergy {
			klog.Infof("Node %s filtered out due to high energy consumption: %f", nodeInfo.Node().Name, nodeEnergy)
			return framework.NewStatus(framework.Unschedulable, fmt.Sprintf("Node %s exceeds energy consumption threshold", nodeInfo.Node().Name))
		}
		klog.Infof("Node %s passed energy filter with consumption: %f", nodeInfo.Node().Name, nodeEnergy)
		return framework.NewStatus(framework.Success)
	}

	klog.Infof("Node %s filtered out because energy consumption data is not available", nodeInfo.Node().Name)
	return framework.NewStatus(framework.Unschedulable, fmt.Sprintf("Node %s does not have energy consumption data", nodeInfo.Node().Name))
}

func New(_ runtime.Object, handle framework.Handle) (framework.Plugin, error) {
    klog.Infof("Initializing %s plugin", PluginName)
    clientset, err := getKubernetesClient()
    if err != nil {
        return nil, err
    }

    var pc *keplerpower.PrometheusClient
    var rpc *edgepower.RaspberryPiPrometheusClient
	nodeIPMap := make(map[string]string)

    if isKeplerDeployed(clientset) {
        klog.Infof("Kepler is deployed, using Kepler metrics.")
        pc, err = keplerpower.NewPrometheusClient()
        if err != nil {
            return nil, err
        }
    } else {
        klog.Infof("Kepler is not deployed, using Raspberry Pi metrics.")
        rpc, err = edgepower.NewRaspberryPiPrometheusClient()
        if err != nil {
            return nil, err
        }

        // Map node names to their IPs
        nodes, err := clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
        if err != nil {
            return nil, fmt.Errorf("error listing nodes: %v", err)
        }

        for _, node := range nodes.Items {
            for _, addr := range node.Status.Addresses {
                if addr.Type == v1.NodeInternalIP {
                    nodeIPMap[node.Name] = addr.Address
                }
            }
        }
    }

    return &NodeSelectorPlugin{
        handle:   handle,
        pc:       pc,
        rpc:      rpc,
        nodeIPMap: nodeIPMap,
    }, nil
}

func (pl *NodeSelectorPlugin) getAllNodeEnergies() (map[string]float64, error) {
    energyMetrics := make(map[string]float64)
    var err error

    if pl.pc != nil {
        energyMetrics, err = pl.pc.GetAllNodeEnergyConsumptions()
    } else if pl.rpc != nil {
        energyMetrics, err = pl.rpc.GetAllRaspberryPiNodeEnergies()
    } else {
        klog.Errorf("Neither Kepler nor Raspberry Pi Prometheus client is initialized")
        return nil, fmt.Errorf("no Prometheus client available")
    }

    return energyMetrics, err
}

func getKubernetesClient() (*kubernetes.Clientset, error) {
    var config *rest.Config
    var err error

    if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
        config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
    } else {
        config, err = rest.InClusterConfig()
    }

    if err != nil {
        return nil, err
    }

    return kubernetes.NewForConfig(config)
}

func isKeplerDeployed(clientset *kubernetes.Clientset) bool {
	_, err := clientset.CoreV1().Namespaces().Get(context.TODO(), "kepler", metav1.GetOptions{})
	if err != nil {
		klog.Infof("Kepler namespace not found: %v", err)
		return false
	}
	return true
}

func getNodeIP(nodeName string) string {
    clientset, err := getKubernetesClient()
    if err != nil {
        klog.Errorf("Error creating Kubernetes client: %v", err)
        return ""
    }

    nodes, err := clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
    if err != nil {
        klog.Errorf("Error listing nodes: %v", err)
        return ""
    }

    for _, node := range nodes.Items {
        if node.Name == nodeName {
            for _, addr := range node.Status.Addresses {
                if addr.Type == v1.NodeInternalIP {
                    return addr.Address
                }
            }
        }
    }
    return ""
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
