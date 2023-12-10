package advisor

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	prommodel "github.com/prometheus/common/model"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	promOperatorClusterURL = "%s/api/v1/namespaces/%s/services/prometheus-operated:%s/proxy/"
	podCPURequest          = `quantile_over_time(%s, node_namespace_pod_container:container_cpu_usage_seconds_total:%s{pod="%s", container!=""}[1w])`
	podCPULimit            = `max_over_time(node_namespace_pod_container:container_cpu_usage_seconds_total:%s{pod="%s", container!=""}[1w]) * %s`
	podMemoryRequest       = `quantile_over_time(%s, container_memory_working_set_bytes{pod="%s", container!=""}[1w]) / 1024 / 1024`
	podMemoryLimit         = `(max_over_time(container_memory_working_set_bytes{pod="%s", container!=""}[1w]) / 1024 / 1024) * %s`
	deploymentRevision     = "deployment.kubernetes.io/revision"
)

func findConfig() (*rest.Config, string, error) {
	cfg, err := clientcmd.NewDefaultClientConfigLoadingRules().Load()
	if err != nil {
		return nil, "", err
	}
	namespace := ""
	for k, v := range cfg.Contexts {
		if cfg.CurrentContext == k {
			namespace = v.Namespace
			break
		}
	}
	conf, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{}).ClientConfig()
	return conf, namespace, err
}

func newClientSet() (*kubernetes.Clientset, error) {
	config, _, err := findConfig()
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(config)
}

func queryStatistic(ctx context.Context, client *promClient, request string, now time.Time) (map[string]float64, error) {
	output := make(map[string]float64)
	response, err := queryPrometheus(ctx, client, request, now)
	if err != nil {
		return output, fmt.Errorf("error querying statistic %w", err)
	}
	asSamples, ok := response.(prommodel.Vector)
	if !ok {
		return output, fmt.Errorf("error converting response to vector")
	}
	sampleArray := []*prommodel.Sample{}
	for _, sample := range asSamples {
		sampleArray = append(sampleArray, sample)
	}

	highest := float64(0.00)
	for _, item := range sampleArray {
		containerName := ""
		for k, v := range item.Metric {
			if k == "container" {
				containerName = string(v)
			}
		}
		if float64(item.Value) > highest {
			output[containerName] = float64(item.Value)
			highest = float64(item.Value)
		}
	}

	return output, nil
}

func (o *Options) queryPrometheusForPod(ctx context.Context, client *promClient, pod v1.Pod) (prometheusMetrics, error) {
	now := time.Now()
	var err error

	output := prometheusMetrics{}
	output.RequestCPU, err = queryStatistic(ctx, client, fmt.Sprintf(podCPURequest, o.Quantile, o.mode, pod.Name), now)
	if err != nil {
		return output, err
	}

	output.LimitCPU, err = queryStatistic(ctx, client, fmt.Sprintf(podCPULimit, o.mode, pod.Name, o.LimitMargin), now)
	if err != nil {
		return output, err
	}

	output.RequestMem, err = queryStatistic(ctx, client, fmt.Sprintf(podMemoryRequest, o.Quantile, pod.Name), now)
	if err != nil {
		return output, err
	}

	output.LimitMem, err = queryStatistic(ctx, client, fmt.Sprintf(podMemoryLimit, pod.Name, o.LimitMargin), now)
	if err != nil {
		return output, err
	}

	return output, nil
}

func float64Peak(input []float64) float64 {
	highest := float64(0.00)
	for _, value := range input {
		if value > highest {
			highest = value
		}
	}
	return highest
}

func findReplicaset(replicasets *appsv1.ReplicaSetList, dep appsv1.Deployment) (*appsv1.ReplicaSet, error) {
	generation, ok := dep.Annotations[deploymentRevision]
	if !ok {
		return nil, fmt.Errorf("could not find label %s for deployment '%s'", deploymentRevision, dep.Name)
	}
	for _, replicaset := range replicasets.Items {
		val, ok := replicaset.Annotations[deploymentRevision]
		if ok && val == generation {
			return &replicaset, nil
		}
	}
	return nil, fmt.Errorf("could not find replicaset for deployment '%s' gen '%v'", dep.Name, generation)
}

func makePrometheusClientForCluster(namespace string, portname string) (*promClient, error) {
	config, _, err := findConfig()
	if err != nil {
		return nil, err
	}

	promurl := fmt.Sprintf(promOperatorClusterURL, config.Host, namespace, portname)
	transport, err := rest.TransportFor(config)
	if err != nil {
		return nil, err
	}

	var httpClient *http.Client
	if transport != http.DefaultTransport {
		httpClient = &http.Client{Transport: transport}
		if config.Timeout > 0 {
			httpClient.Timeout = config.Timeout
		}
	}

	u, err := url.Parse(promurl)
	if err != nil {
		return nil, err
	}
	u.Path = strings.TrimRight(u.Path, "/")

	return &promClient{
		endpoint: u,
		client:   httpClient,
	}, nil
}

func queryPrometheus(ctx context.Context, client *promClient, query string, ts time.Time) (interface{}, error) {
	promcli := promv1.NewAPI(client)
	result, _, err := promcli.Query(ctx, query, ts)
	return result, err
}

func (o *Options) detectMode(ctx context.Context) (string, error) {
	now := time.Now()

	cpuUsage := `node_namespace_pod_container:container_cpu_usage_seconds_total:%s`

	request := fmt.Sprintf(cpuUsage, "sum_irate")
	response, err := queryPrometheus(ctx, o.promClient, request, now)
	if err != nil {
		return "", fmt.Errorf("error detecting mode %w", err)
	}
	asSamples, ok := response.(prommodel.Vector)
	if !ok {
		return "", fmt.Errorf("error converting response to vector")
	}

	if len(asSamples) > 0 {
		return "sum_irate", nil
	}

	request = fmt.Sprintf(cpuUsage, "sum_rate")
	response, err = queryPrometheus(ctx, o.promClient, request, now)
	if err != nil {
		return "", fmt.Errorf("error detecting mode %w", err)
	}
	asSamples, ok = response.(prommodel.Vector)
	if !ok {
		return "", fmt.Errorf("error converting response to vector")
	}
	if len(asSamples) > 0 {
		return "sum_rate", nil
	}
	return "", fmt.Errorf("could not find cpu mode")
}

func (c *promClient) URL(ep string, args map[string]string) *url.URL {
	p := path.Join(c.endpoint.Path, ep)

	for arg, val := range args {
		arg = ":" + arg
		p = strings.ReplaceAll(p, arg, val)
	}

	u := *c.endpoint
	u.Path = p

	return &u
}

func (c *promClient) Do(ctx context.Context, req *http.Request) (*http.Response, []byte, error) {
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	resp, err := c.client.Do(req)
	defer func() {
		if resp != nil {
			resp.Body.Close()
		}
	}()

	if err != nil {
		return nil, nil, err
	}

	var body []byte
	done := make(chan struct{})
	go func() {
		body, err = io.ReadAll(resp.Body)
		close(done)
	}()

	select {
	case <-ctx.Done():
		<-done
		err = resp.Body.Close()
		if err == nil {
			err = ctx.Err()
		}
	case <-done:
	}

	return resp, body, err
}

func byteCountSI(b int64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB",
		float64(b)/float64(div), "kMGTPE"[exp])
}

func (o *Options) findPods(ctx context.Context, namespace string, selector string) (prometheusMetrics, error) {
	final := prometheusMetrics{
		LimitCPU:   make(map[string]float64),
		LimitMem:   make(map[string]float64),
		RequestCPU: make(map[string]float64),
		RequestMem: make(map[string]float64),
	}

	pods, err := o.Client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return final, err
	}

	totalLimitCPU := make(map[string][]float64)
	totalLimitMem := make(map[string][]float64)
	totalRequestCPU := make(map[string][]float64)
	totalRequestMem := make(map[string][]float64)

	for _, pod := range pods.Items {
		output, err := o.queryPrometheusForPod(ctx, o.promClient, pod)
		if err != nil {
			return final, err
		}
		for k, v := range output.RequestCPU {
			totalRequestCPU[k] = append(totalRequestCPU[k], v)
		}
		for k, v := range output.RequestMem {
			totalRequestMem[k] = append(totalRequestMem[k], v)
		}
		for k, v := range output.LimitCPU {
			totalLimitCPU[k] = append(totalLimitCPU[k], v)
		}
		for k, v := range output.LimitMem {
			totalLimitMem[k] = append(totalLimitMem[k], v)
		}
	}

	for k, v := range totalRequestCPU {
		scale := 10
		value := float64Peak(v)
		if value < 0.01 {
			scale = 100
		}
		final.RequestCPU[k] = math.Ceil(value*float64(scale)) / float64(scale)
	}
	for k, v := range totalRequestMem {
		final.RequestMem[k] = math.Ceil(float64Peak(v)/100) * 100
	}
	for k, v := range totalLimitCPU {
		scale := 10
		value := float64Peak(v)
		if value < 0.01 {
			scale = 100
		}
		final.LimitCPU[k] = math.Ceil(value*float64(scale)) / float64(scale)
	}
	for k, v := range totalLimitMem {
		final.LimitMem[k] = math.Ceil(float64Peak(v)/100) * 100
	}
	return final, nil
}

func buildUsedNamespaces(ctx context.Context, o *Options) (string, error) {
	if o.NamespaceSelector != "" {
		namespaces, err := o.Client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{
			LabelSelector: o.NamespaceSelector,
		})
		if err != nil {
			return "", err
		}
		strNamespace := []string{}
		for _, name := range namespaces.Items {
			strNamespace = append(strNamespace, name.Name)
		}
		return strings.Join(strNamespace, ","), nil
	} else if o.Namespaces != "" {
		return o.Namespaces, nil
	}
	_, namespace, err := findConfig()
	if err != nil {
		return "", err
	}
	return namespace, nil
}

func makeClientForCluster(ctx context.Context, o *Options) (*promClient, error) {
	promService, err := o.Client.CoreV1().Services("").List(ctx, metav1.ListOptions{
		LabelSelector: "operated-prometheus=true",
	})
	if err != nil {
		return nil, err
	}
	if len(promService.Items) == 0 || len(promService.Items[0].Spec.Ports) == 0 {
		return nil, fmt.Errorf("prometheus-operator not detected")
	}
	return makePrometheusClientForCluster(promService.Items[0].Namespace, promService.Items[0].Spec.Ports[0].Name)
}

func (o *Options) handleDeployments(ctx context.Context, namespace string, data [][]string) ([][]string, float64, float64, error) {
	deployments, err := o.Client.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, 0, 0, err
	}

	totalCPUSave := float64(0.00)
	totalMemSave := float64(0.00)

	for _, deployment := range deployments.Items {
		selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
		if err != nil {
			return nil, 0, 0, err
		}

		replicasets, err := o.Client.AppsV1().ReplicaSets(deployment.Namespace).List(ctx, metav1.ListOptions{
			LabelSelector: selector.String(),
		})
		if err != nil {
			return nil, 0, 0, err
		}

		replicaset, err := findReplicaset(replicasets, deployment)
		if err != nil {
			return nil, 0, 0, err
		}

		selector, err = metav1.LabelSelectorAsSelector(replicaset.Spec.Selector)
		if err != nil {
			return nil, 0, 0, err
		}

		final, err := o.findPods(ctx, deployment.Namespace, selector.String())
		if err != nil {
			return nil, 0, 0, err
		}

		var cpuSave float64
		var memSave float64
		data, cpuSave, memSave = o.analyzeDeployment(data, deployment, final)
		totalCPUSave += cpuSave
		totalMemSave += memSave
	}
	return data, totalCPUSave, totalMemSave, nil
}

func (o *Options) handleStatefulsets(ctx context.Context, namespace string, data [][]string) ([][]string, float64, float64, error) {
	statefulSets, err := o.Client.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, 0, 0, err
	}

	totalCPUSave := float64(0.00)
	totalMemSave := float64(0.00)

	for _, statefulSet := range statefulSets.Items {
		selector, err := metav1.LabelSelectorAsSelector(statefulSet.Spec.Selector)
		if err != nil {
			return nil, 0, 0, err
		}

		final, err := o.findPods(ctx, statefulSet.Namespace, selector.String())
		if err != nil {
			return nil, 0, 0, err
		}

		var cpuSave float64
		var memSave float64
		data, cpuSave, memSave = o.analyzeStatefulset(data, statefulSet, final)
		totalCPUSave += cpuSave
		totalMemSave += memSave
	}
	return data, totalCPUSave, totalMemSave, nil
}

func (o *Options) handleDaemonsets(ctx context.Context, namespace string, data [][]string) ([][]string, float64, float64, error) {
	daemonSets, err := o.Client.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, 0, 0, err
	}

	totalCPUSave := float64(0.00)
	totalMemSave := float64(0.00)

	for _, daemonSets := range daemonSets.Items {
		selector, err := metav1.LabelSelectorAsSelector(daemonSets.Spec.Selector)
		if err != nil {
			return nil, 0, 0, err
		}

		final, err := o.findPods(ctx, daemonSets.Namespace, selector.String())
		if err != nil {
			return nil, 0, 0, err
		}

		var cpuSave float64
		var memSave float64
		data, cpuSave, memSave = o.analyzeDaemonSet(data, daemonSets, final)
		totalCPUSave += cpuSave
		totalMemSave += memSave
	}
	return data, totalCPUSave, totalMemSave, nil
}
