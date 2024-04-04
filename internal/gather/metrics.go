package gather

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	prometheus "github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	_ "github.com/prometheus/common/model"

	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/internal/pods"
	"github.com/submariner-io/subctl/pkg/cluster"
	"github.com/submariner-io/subctl/pkg/image"
	"k8s.io/client-go/kubernetes"
	"time"
)

func spawnClientPodOnNonGatewayNode(client kubernetes.Interface, namespace, podCommand string,
	imageRepInfo *image.RepositoryInfo,
) (*pods.Scheduled, error) {
	scheduling := pods.Scheduling{ScheduleOn: pods.NonGatewayNode, Networking: pods.PodNetworking}
	return spawnPod(client, scheduling, "gather-metrics-data", namespace, podCommand, imageRepInfo)
}

func spawnPod(client kubernetes.Interface, scheduling pods.Scheduling, podName, namespace,
	podCommand string, imageRepInfo *image.RepositoryInfo,
) (*pods.Scheduled, error) {
	pod, err := pods.ScheduleSubctlPod(&pods.Config{
		Name:                podName,
		ClientSet:           client,
		Scheduling:          scheduling,
		Namespace:           namespace,
		Command:             podCommand,
		ImageRepositoryInfo: *imageRepInfo,
		ServiceAccountName:  "submariner-diagnose",
	})
	if err != nil {
		return nil, errors.Wrap(err, "error scheduling pod")
	}

	return pod, nil
}

func DataFromMetrics(info *Info) {
	var imageOverrides = []string{"submariner-subctl=localhost:5000/subctl:local"}
	repositoryInfo, err := info.Info.GetImageRepositoryInfo(imageOverrides...)

	fmt.Println("Collecting metrics data")

	if err != nil {
		fmt.Printf("Error spawning the client pod. %v\n", err.Error())
		return
	}

	cPod, err := spawnClientPodOnNonGatewayNode(info.ClientProducer.ForKubernetes(),
		constants.OperatorNamespace, "subctl gather-metrics --in-cluster", repositoryInfo)
	if err != nil {
		fmt.Printf("Error spawning the client pod. %v\n", err.Error())
		return
	}

	defer cPod.Delete()

	if err = cPod.AwaitCompletion(); err != nil {
		return
	}
	gatherPodLogs(gatherMetricsPodLabel, info)

}

func collectConnectionsDataFromMetrics(clusterInfo *cluster.Info, options Options, info *Info) {
	//prometheusURL := "http://prometheus-operated.default.svc.cluster.local:9090" ==> works with kind setup
	prometheusURL := "https://prometheus-k8s.openshift-monitoring.svc.cluster.local:9091" //==> TODO: check if running under OCP or not to set the URL.

	fmt.Printf("prometheusURL: %v\n", prometheusURL)

	client, err := prometheus.NewClient(prometheus.Config{
		Address: prometheusURL,
	})
	if err != nil {
		fmt.Println("Error creating Prometheus client:", err)
		return
	}

	apiClient := promv1.NewAPI(client)
	query := "submariner_connections"

	// Set the time range for the query (optional) - how long back to get the data from
	startTime := time.Now().Add(-10 * time.Hour)
	endTime := time.Now()

	result, warnings, err := apiClient.QueryRange(context.Background(), query, promv1.Range{
		Start: startTime,
		End:   endTime,
		Step:  60 * time.Second,
	})
	if err != nil {
		fmt.Println("Error executing query:", err)
		return
	}

	if len(warnings) > 0 {
		fmt.Println("Query warnings:", warnings)
	}

	switch result.(type) {
	case model.Value:
		vectorVal := result.(model.Matrix)
		for _, elem := range vectorVal {
			labels := elem.Metric
			statusValue := labels["status"]
			t1 := elem.Values[0].Timestamp
			t2 := elem.Values[len(elem.Values)-1].Timestamp

			if statusValue != "connected" {
				fmt.Printf("Warning! ")
			}
			fmt.Printf("Connection with cabel driver %v from %v{%v} to %v{%v} was in status %v from %v to %v\n",
				labels["cable_driver"], labels["local_endpoint_ip"], labels["local_cluster"], labels["remote_endpoint_ip"], labels["remote_cluster"], statusValue,
				t1.Time().UTC().Format("2006-01-02 15:04:05"),
				t2.Time().UTC().Format("2006-01-02 15:04:05"))

		}
	default:
		fmt.Println("default.. ->")
		fmt.Println(result)
	}
}
