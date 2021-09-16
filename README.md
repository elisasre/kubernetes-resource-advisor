# resource-advisor

### Installation

Mac (check other operating system download urls under releases):

```
curl -Lo /usr/local/bin/kubectl-advisory https://github.com/ElisaOyj/kubernetes-resource-advisor/releases/download/1.0.3/resource-advisor-darwin-amd64
```

You need to store the binary under PATH to make it usable as kubectl plugin.

### Usage

```
kubectl advisory --help
I0916 13:28:02.492261   96935 start.go:23] Starting application...
Kubernetes resource-advisor

Usage:
  resource-advisor [flags]

Flags:
  -h, --help                        help for resource-advisor
      --limit-margin string         Limit margin (default "1.2")
      --namespace-selector string   Namespace selector
      --namespaces string           Comma separated namespaces to be scanned
      --quantile string             Quantile to be used (default "0.95")
```

```
% kubectl advisory
I0916 13:26:56.442083   96863 start.go:23] Starting application...
Namespaces: logging
Quantile: 0.95
Limit margin: 1.2
+-----------+----------------------+------------+--------------------+--------------------+------------------+------------------+
| NAMESPACE |       RESOURCE       | CONTAINER  | REQUEST CPU (SPEC) | REQUEST MEM (SPEC) | LIMIT CPU (SPEC) | LIMIT MEM (SPEC) |
+-----------+----------------------+------------+--------------------+--------------------+------------------+------------------+
| logging   | daemonset/fluent-bit | fluent-bit | 10m (25m)          | 100Mi (100Mi)      | 100m (400m)      | 100Mi (200Mi)    |
+-----------+----------------------+------------+--------------------+--------------------+------------------+------------------+
Total savings:
You could save 0.32 vCPUs and 102.0 MB Memory by changing the settings
```

What these numbers mean? The idea of this tool is to find out `quantile` (default is 95%) CPU & memory usage for single pod. We use that usage for specifying `requests`. Then there is another value called `limit-margin` which is used for specifying `limits`. The default settings means that 95% of time the POD has quarantee for the resources, and 5% of time it uses burstable capacity between 95% -> 120% of POD maximum usage in history.

### Motivation

As SRE team we are seeing all the time Kubernetes clusters in which developers are requesting too much / too low amount of CPU or memory to PODs. In big environments this can lead to huge overhead - PODs are requesting the CPU/mem but not using it. That was motivation for this tool, by this tool we can check the real usage of CPU/memory of pod and change the requests/limits accordingly.
