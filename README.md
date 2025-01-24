# Kubernetes Resource Advisor

## Installation

```bash
export PLUGIN_VERSION=x.y.z

# MacOS (x86_64)
curl -fsSLo /usr/local/bin/kubectl-advisory "https://github.com/elisasre/kubernetes-resource-advisor/releases/download/${PLUGIN_VERSION}/resource-advisor-darwin-amd64"

# MacOS (M1)
curl -fsSLo /usr/local/bin/kubectl-advisory "https://github.com/elisasre/kubernetes-resource-advisor/releases/download/${PLUGIN_VERSION}/resource-advisor-darwin-arm64"

# Linux (x86_64)
curl -fsSLo /usr/local/bin/kubectl-advisory "https://github.com/elisasre/kubernetes-resource-advisor/releases/download/${PLUGIN_VERSION}/resource-advisor-linux-amd64"

# All
chmod 755 /usr/local/bin/kubectl-advisory
```

You need to store the binary under PATH to make it usable as kubectl plugin.

## Requirements

The [prometheus-operator](https://github.com/prometheus-operator/prometheus-operator) is required in the target Kubernetes cluster to provide necessary metrics.

## Usage

```bash
% kubectl advisory --help
Kubernetes resource-advisor

Usage:
  resource-advisor [flags]

Flags:
  -h, --help                        help for resource-advisor
  -m, --limit-margin string         Limit margin (default "1.2")
  -l, --namespace-selector string   Namespace selector
  -n, --namespaces string           Comma separated namespaces to be scanned
  -q, --quantile string             Quantile to be used (default "0.95")
```

```bash
% kubectl advisory
Namespaces: logging
Quantile: 0.95
Limit margin: 1.2
Using mode: sum_irate
+-----------+----------------------+------------+--------------------+--------------------+------------------+------------------+
| NAMESPACE |       RESOURCE       | CONTAINER  | REQUEST CPU (SPEC) | REQUEST MEM (SPEC) | LIMIT CPU (SPEC) | LIMIT MEM (SPEC) |
+-----------+----------------------+------------+--------------------+--------------------+------------------+------------------+
| logging   | daemonset/fluent-bit | fluent-bit | 10m (25m)          | 100Mi (100Mi)      | 100m (400m)      | 200Mi (200Mi)    |
+-----------+----------------------+------------+--------------------+--------------------+------------------+------------------+
Total savings:
You could save 0.27 vCPUs and 87.4 MB Memory by changing the settings
```

What these numbers mean? The idea of this tool is to find out `quantile` (default is 95%) CPU & memory real usage for single POD using Prometheus operator. We use that real usage value for specifying `requests`. Then there is another variable called `limit-margin` which is used for specifying `limits`. The default settings means that 95% of time the POD has quarantee for the resources, and 5% of time it uses burstable capacity between 95% -> 120% of POD maximum usage in history.

### Using namespace-selector

```bash
% kubectl advisory --namespace-selector maintainer=a_crowd_devops
Namespaces: actions-runner-system,cert-manager,default,gha,kaas-test-infra
Quantile: 0.95
Limit margin: 1.2
Using mode: sum_irate
+-----------------------+-------------------------------------------------+-----------------+--------------------+--------------------+------------------+------------------+
|       NAMESPACE       |                    RESOURCE                     |    CONTAINER    | REQUEST CPU (SPEC) | REQUEST MEM (SPEC) | LIMIT CPU (SPEC) | LIMIT MEM (SPEC) |
+-----------------------+-------------------------------------------------+-----------------+--------------------+--------------------+------------------+------------------+
| actions-runner-system | deployment/gha-exporter                         | gha-exporter    | 10m (<nil>)        | 100Mi (<nil>)      | 10m (<nil>)      | 100Mi (<nil>)    |
| actions-runner-system | deployment/ghe-runner-actions-runner-controller | manager         | 10m (<nil>)        | 100Mi (<nil>)      | 10m (<nil>)      | 100Mi (<nil>)    |
| actions-runner-system | deployment/ghe-runner-actions-runner-controller | kube-rbac-proxy | 10m (<nil>)        | 100Mi (<nil>)      | 10m (<nil>)      | 100Mi (<nil>)    |
| cert-manager          | deployment/cert-manager                         | cert-manager    | 10m (<nil>)        | 100Mi (<nil>)      | 10m (<nil>)      | 100Mi (<nil>)    |
| cert-manager          | deployment/cert-manager-cainjector              | cert-manager    | 10m (<nil>)        | 100Mi (<nil>)      | 10m (<nil>)      | 200Mi (<nil>)    |
| cert-manager          | deployment/cert-manager-webhook                 | cert-manager    | 10m (<nil>)        | 100Mi (<nil>)      | 10m (<nil>)      | 100Mi (<nil>)    |
| kaas-test-infra       | deployment/kaas-test-infra                      | kaas-test-infra | 10m (100m)         | 100Mi (200Mi)      | 10m (400m)       | 100Mi (600Mi)    |
+-----------------------+-------------------------------------------------+-----------------+--------------------+--------------------+------------------+------------------+
Total savings:
You could save 0.12 vCPUs and -380.6 MB Memory by changing the settings
```

### Using as library

```go
import "github.com/elisasre/kubernetes-resource-advisor/pkg/advisor"

response, err := advisor.Run(&advisor.Options{
    Namespaces: "logging,monitoring",
})
```

## Motivation

As SRE team we are seeing all the time Kubernetes clusters in which developers are requesting too much / too low amount of CPU or memory to PODs. In big environments this can lead to huge overhead - PODs are requesting the CPU/mem but not using it. That was motivation for this tool, by this tool we can check the real usage of CPU/memory of pod and change the requests/limits accordingly.
