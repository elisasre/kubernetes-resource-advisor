package advisor

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/olekukonko/tablewriter"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/api/core/v1"
	apresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Run(o *Options) error {
	var err error
	o.client, err = newClientSet()
	if err != nil {
		return err
	}

	o.promClient, err = makePrometheusClientForCluster()
	if err != nil {
		return err
	}
	ctx := context.Background()

	if o.NamespaceSelector != "" {
		namespaces, err := o.client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{
			LabelSelector: o.NamespaceSelector,
		})
		if err != nil {
			return err
		}

		strNamespace := []string{}
		for _, name := range namespaces.Items {
			strNamespace = append(strNamespace, name.Name)
		}
		o.Namespaces = strings.Join(strNamespace, ",")
	} else if o.NamespaceInput != "" {
		o.Namespaces = o.NamespaceInput
	} else {
		_, namespace, err := findConfig()
		if err != nil {
			return err
		}
		o.Namespaces = namespace
	}

	fmt.Printf("Namespaces: %s\n", o.Namespaces)
	fmt.Printf("Quantile: %s\n", o.Quantile)
	fmt.Printf("Limit margin: %s\n", o.LimitMargin)

	data := [][]string{}

	totalCPUSave := float64(0.00)
	totalMemSave := float64(0.00)
	for _, namespace := range strings.Split(o.Namespaces, ",") {
		deployments, err := o.client.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return err
		}

		for _, deployment := range deployments.Items {
			selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
			if err != nil {
				return err
			}

			replicasets, err := o.client.AppsV1().ReplicaSets(deployment.Namespace).List(ctx, metav1.ListOptions{
				LabelSelector: selector.String(),
			})
			if err != nil {
				return err
			}

			replicaset, err := findReplicaset(replicasets, deployment)
			if err != nil {
				return err
			}

			selector, err = metav1.LabelSelectorAsSelector(replicaset.Spec.Selector)
			if err != nil {
				return err
			}

			final, err := o.findPods(ctx, deployment.Namespace, selector.String())
			if err != nil {
				return err
			}

			cpuSave := float64(0.00)
			memSave := float64(0.00)
			data, cpuSave, memSave = o.analyzeDeployment(data, deployment, final)
			totalCPUSave += cpuSave
			totalMemSave += memSave
		}

		statefulSets, err := o.client.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return err
		}

		for _, statefulSet := range statefulSets.Items {
			selector, err := metav1.LabelSelectorAsSelector(statefulSet.Spec.Selector)
			if err != nil {
				return err
			}

			final, err := o.findPods(ctx, statefulSet.Namespace, selector.String())
			if err != nil {
				return err
			}

			cpuSave := float64(0.00)
			memSave := float64(0.00)
			data, cpuSave, memSave = o.analyzeStatefulset(data, statefulSet, final)
			totalCPUSave += cpuSave
			totalMemSave += memSave
		}

		daemonSets, err := o.client.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return err
		}

		for _, daemonSets := range daemonSets.Items {
			selector, err := metav1.LabelSelectorAsSelector(daemonSets.Spec.Selector)
			if err != nil {
				return err
			}

			final, err := o.findPods(ctx, daemonSets.Namespace, selector.String())
			if err != nil {
				return err
			}

			cpuSave := float64(0.00)
			memSave := float64(0.00)
			data, cpuSave, memSave = o.analyzeDaemonSet(data, daemonSets, final)
			totalCPUSave += cpuSave
			totalMemSave += memSave
		}
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Namespace", "Resource", "Container", "Request CPU (spec)", "Request MEM (spec)", "Limit CPU (spec)", "Limit MEM (spec)"})
	for _, v := range data {
		table.Append(v)
	}
	table.Render()

	fmt.Printf("Total savings:\n")

	totalMem := int64(totalMemSave)
	totalMemStr := ByteCountSI(totalMem)
	if totalMem < 0 {
		totalMem *= -1
		totalMemStr = ByteCountSI(totalMem)
		totalMemStr = fmt.Sprintf("-%s", totalMemStr)
	}
	fmt.Printf("You could save %.2f vCPUs and %s Memory by changing the settings\n", totalCPUSave, totalMemStr)

	return nil
}

func currentValue(resources v1.ResourceRequirements, method string, resource v1.ResourceName, current int, format apresource.Format) (float64, string) {
	curSaving := float64(float64(current) * 1000 * 1000)
	if format == apresource.DecimalSI {
		curSaving = float64(float64(current) / 1000)
	}

	if method == "limit" {
		val, ok := resources.Limits[resource]
		if ok {
			return val.AsApproximateFloat64() - curSaving, val.String()
		}
	} else {
		val, ok := resources.Requests[resource]
		if ok {
			return val.AsApproximateFloat64() - curSaving, val.String()
		}
	}
	return -1 * curSaving, "<nil>"
}

func (o *Options) analyzeDaemonSet(data [][]string, daemonset appsv1.DaemonSet, finalMetrics prometheusMetrics) ([][]string, float64, float64) {
	totalCPUSavings := float64(0.00)
	totalMemSavings := float64(0.00)
	for _, container := range daemonset.Spec.Template.Spec.Containers {
		reqCpu := int(finalMetrics.RequestCPU[container.Name] * 1000)
		reqMem := int(finalMetrics.RequestMem[container.Name])
		limCpu := int(finalMetrics.LimitCPU[container.Name] * 1000)
		limMem := int(finalMetrics.LimitMem[container.Name])

		reqCpuSave, strReqCPU := currentValue(container.Resources, "request", v1.ResourceCPU, reqCpu, apresource.DecimalSI)
		reqMemSave, strReqMem := currentValue(container.Resources, "request", v1.ResourceMemory, reqMem, apresource.BinarySI)
		_, strLimCPU := currentValue(container.Resources, "limit", v1.ResourceCPU, limCpu, apresource.DecimalSI)
		_, strLimMem := currentValue(container.Resources, "limit", v1.ResourceMemory, limMem, apresource.BinarySI)

		totalCPUSavings += reqCpuSave * float64(daemonset.Status.DesiredNumberScheduled)
		totalMemSavings += reqMemSave * float64(daemonset.Status.DesiredNumberScheduled)
		data = append(data, []string{
			daemonset.Namespace,
			fmt.Sprintf("daemonset/%s", daemonset.Name),
			container.Name,
			fmt.Sprintf("%dm (%s)", reqCpu, strReqCPU),
			fmt.Sprintf("%dMi (%s)", reqMem, strReqMem),
			fmt.Sprintf("%dm (%s)", limCpu, strLimCPU),
			fmt.Sprintf("%dMi (%s)", limMem, strLimMem),
		})
	}
	return data, totalCPUSavings, totalMemSavings
}

func (o *Options) analyzeStatefulset(data [][]string, statefulset appsv1.StatefulSet, finalMetrics prometheusMetrics) ([][]string, float64, float64) {
	totalCPUSavings := float64(0.00)
	totalMemSavings := float64(0.00)
	for _, container := range statefulset.Spec.Template.Spec.Containers {
		reqCpu := int(finalMetrics.RequestCPU[container.Name] * 1000)
		reqMem := int(finalMetrics.RequestMem[container.Name])
		limCpu := int(finalMetrics.LimitCPU[container.Name] * 1000)
		limMem := int(finalMetrics.LimitMem[container.Name])

		reqCpuSave, strReqCPU := currentValue(container.Resources, "request", v1.ResourceCPU, reqCpu, apresource.DecimalSI)
		reqMemSave, strReqMem := currentValue(container.Resources, "request", v1.ResourceMemory, reqMem, apresource.BinarySI)
		_, strLimCPU := currentValue(container.Resources, "limit", v1.ResourceCPU, limCpu, apresource.DecimalSI)
		_, strLimMem := currentValue(container.Resources, "limit", v1.ResourceMemory, limMem, apresource.BinarySI)

		totalCPUSavings += reqCpuSave * float64(*statefulset.Spec.Replicas)
		totalMemSavings += reqMemSave * float64(*statefulset.Spec.Replicas)
		data = append(data, []string{
			statefulset.Namespace,
			fmt.Sprintf("statefulset/%s", statefulset.Name),
			container.Name,
			fmt.Sprintf("%dm (%s)", reqCpu, strReqCPU),
			fmt.Sprintf("%dMi (%s)", reqMem, strReqMem),
			fmt.Sprintf("%dm (%s)", limCpu, strLimCPU),
			fmt.Sprintf("%dMi (%s)", limMem, strLimMem),
		})
	}
	return data, totalCPUSavings, totalMemSavings
}

func (o *Options) analyzeDeployment(data [][]string, deployment appsv1.Deployment, finalMetrics prometheusMetrics) ([][]string, float64, float64) {
	totalCPUSavings := float64(0.00)
	totalMemSavings := float64(0.00)
	for _, container := range deployment.Spec.Template.Spec.Containers {
		reqCpu := int(finalMetrics.RequestCPU[container.Name] * 1000)
		reqMem := int(finalMetrics.RequestMem[container.Name])
		limCpu := int(finalMetrics.LimitCPU[container.Name] * 1000)
		limMem := int(finalMetrics.LimitMem[container.Name])

		reqCpuSave, strReqCPU := currentValue(container.Resources, "request", v1.ResourceCPU, reqCpu, apresource.DecimalSI)
		reqMemSave, strReqMem := currentValue(container.Resources, "request", v1.ResourceMemory, reqMem, apresource.BinarySI)
		_, strLimCPU := currentValue(container.Resources, "limit", v1.ResourceCPU, limCpu, apresource.DecimalSI)
		_, strLimMem := currentValue(container.Resources, "limit", v1.ResourceMemory, limMem, apresource.BinarySI)

		totalCPUSavings += reqCpuSave * float64(*deployment.Spec.Replicas)
		totalMemSavings += reqMemSave * float64(*deployment.Spec.Replicas)
		data = append(data, []string{
			deployment.Namespace,
			fmt.Sprintf("deployment/%s", deployment.Name),
			container.Name,
			fmt.Sprintf("%dm (%s)", reqCpu, strReqCPU),
			fmt.Sprintf("%dMi (%s)", reqMem, strReqMem),
			fmt.Sprintf("%dm (%s)", limCpu, strLimCPU),
			fmt.Sprintf("%dMi (%s)", limMem, strLimMem),
		})
	}
	return data, totalCPUSavings, totalMemSavings
}
