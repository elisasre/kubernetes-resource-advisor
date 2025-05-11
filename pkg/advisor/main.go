package advisor

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/olekukonko/tablewriter"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apresource "k8s.io/apimachinery/pkg/api/resource"
)

func (o *Options) loadDefaults() {
	if o.Quantile == "" {
		o.Quantile = "0.95"
	}
	if o.LimitMargin == "" {
		o.LimitMargin = "1.2"
	}
}

// Run executes the resource advisor.
func Run(o *Options) (*Response, error) {
	o.loadDefaults()
	var err error
	if o.Client == nil {
		o.Client, err = newClientSet()
		if err != nil {
			return nil, err
		}
	}

	ctx := context.Background()
	o.promClient, err = makeClientForCluster(ctx, o)
	if err != nil {
		return nil, err
	}

	o.mode, err = o.detectMode(ctx)
	if err != nil {
		return nil, err
	}

	o.usedNamespaces, err = buildUsedNamespaces(ctx, o)
	if err != nil {
		return nil, err
	}

	fmt.Printf("Namespaces: %s\n", o.usedNamespaces)
	fmt.Printf("Quantile: %s\n", o.Quantile)
	fmt.Printf("Limit margin: %s\n", o.LimitMargin)
	fmt.Printf("Using mode: %s\n", o.mode)

	data := [][]string{}

	totalCPUSave := float64(0.00)
	totalMemSave := float64(0.00)
	for _, namespace := range strings.Split(o.usedNamespaces, ",") {
		var cpuSave, memSave float64
		data, cpuSave, memSave, err = o.handleDeployments(ctx, namespace, data)
		if err != nil {
			return nil, err
		}
		totalCPUSave += cpuSave
		totalMemSave += memSave

		data, cpuSave, memSave, err = o.handleStatefulsets(ctx, namespace, data)
		if err != nil {
			return nil, err
		}
		totalCPUSave += cpuSave
		totalMemSave += memSave

		data, cpuSave, memSave, err = o.handleDaemonsets(ctx, namespace, data)
		if err != nil {
			return nil, err
		}
		totalCPUSave += cpuSave
		totalMemSave += memSave
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.Header([]string{"Namespace", "Resource", "Container", "Request CPU (spec)", "Request MEM (spec)", "Limit CPU (spec)", "Limit MEM (spec)"})
	for _, v := range data {
		table.Append(v)
	}
	table.Render()

	fmt.Printf("Total savings:\n")

	totalMem := int64(totalMemSave)
	totalMemStr := byteCountSI(totalMem)
	if totalMem < 0 {
		totalMem *= -1
		totalMemStr = byteCountSI(totalMem)
		totalMemStr = fmt.Sprintf("-%s", totalMemStr)
	}
	fmt.Printf("You could save %.2f vCPUs and %s Memory by changing the settings\n", totalCPUSave, totalMemStr)
	return &Response{
		Data:    data,
		CPUSave: totalCPUSave,
		MemSave: totalMem,
	}, nil
}

func currentValue(resources v1.ResourceRequirements, method string, resource v1.ResourceName, current int, format apresource.Format) (float64, string) {
	curSaving := float64(current) * 1000 * 1000
	if format == apresource.DecimalSI {
		curSaving = float64(current) / 1000
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
		reqCPU := int(finalMetrics.RequestCPU[container.Name] * 1000)
		reqMem := int(finalMetrics.RequestMem[container.Name])
		limCPU := int(finalMetrics.LimitCPU[container.Name] * 1000)
		limMem := int(finalMetrics.LimitMem[container.Name])

		reqCPUSave, strReqCPU := currentValue(container.Resources, "request", v1.ResourceCPU, reqCPU, apresource.DecimalSI)
		reqMemSave, strReqMem := currentValue(container.Resources, "request", v1.ResourceMemory, reqMem, apresource.BinarySI)
		_, strLimCPU := currentValue(container.Resources, "limit", v1.ResourceCPU, limCPU, apresource.DecimalSI)
		_, strLimMem := currentValue(container.Resources, "limit", v1.ResourceMemory, limMem, apresource.BinarySI)

		totalCPUSavings += reqCPUSave * float64(daemonset.Status.DesiredNumberScheduled)
		totalMemSavings += reqMemSave * float64(daemonset.Status.DesiredNumberScheduled)
		data = append(data, []string{
			daemonset.Namespace,
			fmt.Sprintf("daemonset/%s", daemonset.Name),
			container.Name,
			fmt.Sprintf("%dm (%s)", reqCPU, strReqCPU),
			fmt.Sprintf("%dMi (%s)", reqMem, strReqMem),
			fmt.Sprintf("%dm (%s)", limCPU, strLimCPU),
			fmt.Sprintf("%dMi (%s)", limMem, strLimMem),
		})
	}
	return data, totalCPUSavings, totalMemSavings
}

func (o *Options) analyzeStatefulset(data [][]string, statefulset appsv1.StatefulSet, finalMetrics prometheusMetrics) ([][]string, float64, float64) {
	totalCPUSavings := float64(0.00)
	totalMemSavings := float64(0.00)
	for _, container := range statefulset.Spec.Template.Spec.Containers {
		reqCPU := int(finalMetrics.RequestCPU[container.Name] * 1000)
		reqMem := int(finalMetrics.RequestMem[container.Name])
		limCPU := int(finalMetrics.LimitCPU[container.Name] * 1000)
		limMem := int(finalMetrics.LimitMem[container.Name])

		reqCPUSave, strReqCPU := currentValue(container.Resources, "request", v1.ResourceCPU, reqCPU, apresource.DecimalSI)
		reqMemSave, strReqMem := currentValue(container.Resources, "request", v1.ResourceMemory, reqMem, apresource.BinarySI)
		_, strLimCPU := currentValue(container.Resources, "limit", v1.ResourceCPU, limCPU, apresource.DecimalSI)
		_, strLimMem := currentValue(container.Resources, "limit", v1.ResourceMemory, limMem, apresource.BinarySI)

		totalCPUSavings += reqCPUSave * float64(*statefulset.Spec.Replicas)
		totalMemSavings += reqMemSave * float64(*statefulset.Spec.Replicas)
		data = append(data, []string{
			statefulset.Namespace,
			fmt.Sprintf("statefulset/%s", statefulset.Name),
			container.Name,
			fmt.Sprintf("%dm (%s)", reqCPU, strReqCPU),
			fmt.Sprintf("%dMi (%s)", reqMem, strReqMem),
			fmt.Sprintf("%dm (%s)", limCPU, strLimCPU),
			fmt.Sprintf("%dMi (%s)", limMem, strLimMem),
		})
	}
	return data, totalCPUSavings, totalMemSavings
}

func (o *Options) analyzeDeployment(data [][]string, deployment appsv1.Deployment, finalMetrics prometheusMetrics) ([][]string, float64, float64) {
	totalCPUSavings := float64(0.00)
	totalMemSavings := float64(0.00)
	for _, container := range deployment.Spec.Template.Spec.Containers {
		reqCPU := int(finalMetrics.RequestCPU[container.Name] * 1000)
		reqMem := int(finalMetrics.RequestMem[container.Name])
		limCPU := int(finalMetrics.LimitCPU[container.Name] * 1000)
		limMem := int(finalMetrics.LimitMem[container.Name])

		reqCPUSave, strReqCPU := currentValue(container.Resources, "request", v1.ResourceCPU, reqCPU, apresource.DecimalSI)
		reqMemSave, strReqMem := currentValue(container.Resources, "request", v1.ResourceMemory, reqMem, apresource.BinarySI)
		_, strLimCPU := currentValue(container.Resources, "limit", v1.ResourceCPU, limCPU, apresource.DecimalSI)
		_, strLimMem := currentValue(container.Resources, "limit", v1.ResourceMemory, limMem, apresource.BinarySI)

		totalCPUSavings += reqCPUSave * float64(*deployment.Spec.Replicas)
		totalMemSavings += reqMemSave * float64(*deployment.Spec.Replicas)
		data = append(data, []string{
			deployment.Namespace,
			fmt.Sprintf("deployment/%s", deployment.Name),
			container.Name,
			fmt.Sprintf("%dm (%s)", reqCPU, strReqCPU),
			fmt.Sprintf("%dMi (%s)", reqMem, strReqMem),
			fmt.Sprintf("%dm (%s)", limCPU, strLimCPU),
			fmt.Sprintf("%dMi (%s)", limMem, strLimMem),
		})
	}
	return data, totalCPUSavings, totalMemSavings
}
