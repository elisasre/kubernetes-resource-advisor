package advisor

import (
	"net/http"
	"net/url"

	"k8s.io/client-go/kubernetes"
)

type Options struct {
	NamespaceInput    string
	NamespaceSelector string
	Namespaces        string
	Quantile          string
	LimitMargin       string
	promClient        *promClient
	client            *kubernetes.Clientset
}

type promClient struct {
	endpoint *url.URL
	client   *http.Client
}

type suggestion struct {
	OldValue  float64
	NewValue  float64
	Pod       string
	Container string
	Message   string
}

type prometheusMetrics struct {
	LimitCPU   map[string]float64
	LimitMem   map[string]float64
	RequestCPU map[string]float64
	RequestMem map[string]float64
}
