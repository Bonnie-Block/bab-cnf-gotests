package rancpuparameters

import (
	mcfgv1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/k8sreporter"
)

var (
	// ReporterNamespacesToDump tells to reporter from where to collect logs.
	ReporterNamespacesToDump = map[string]string{
		parameters.PerformanceAddonOperatorNamespace: "performance",
		ran.NamespaceTesting:                         "other",
	}
	// ReporterCrds tells to reporter what resources to collect.
	ReporterCrds = []k8sreporter.CRData{
		{Cr: &mcfgv1.MachineConfigPoolList{}},
	}
)

// RAN CPU metric names/prefixes.
const (
	RanCPUMetricOsDaemon       = "ranmetrics_cpu_os_daemon"
	RanCPUMetricInfraPods      = "ranmetrics_cpu_infra_pods"
	RanCPUMetricTotal          = "ranmetrics_cpu_total"
	RanAPIServerRate           = "ranmetrics_apiserver_rate"
	RanAPIServerTotal          = "ranmetrics_apiserver_total"
	RanContainerCount          = "ranmetrics_container_count"
	RanContainerCountBreakdown = "ranmetrics_container_count_breakdown"
)

type PromQueryResponse struct {
	Status string
	Error  string
	Data   struct {
		Result []PromMetric
	}
}

// PromMetric struct to hold an item in prom query result list.
type PromMetric struct {
	Metric map[string]string
	Value  []interface{}
}

type PromQueryResponseRanMetrics struct {
	Status string
	Data   struct {
		ResultType string
		Result     []PromMetric
	}
}

const (
	// Prom query statistic representation for management cpu overhead.
	CPUOverheadStat = "namedprocess_namegroup_cpu_rate{groupname!~\"conmon\"}"
	// Prom query statistic representation for infra pods. Assuming only oslat and stress-ng user pods are running.
	CPUInfraPodsStat = "pod:container_cpu_usage:sum{pod!~\"process-exp.*\",pod!~\"oslat.*\",pod!~\"stress.*\"," +
		"pod!~\"cnfgotestpriv.*\",namespace!~\"workload\"}"
	OsTrendQuery = `max without (sw_version)
	(last_over_time(ranmetrics_cpu_os_daemon_steadyworkload_avg{cluster='%s',
	baseline='true', sw_version=~'%s', duration='%s', formal_test='true'}[%dw]))`
	PodTrendQuery = `max without (sw_version)
	(last_over_time(ranmetrics_cpu_infra_pods_steadyworkload_avg{cluster='%s',
	baseline='true', sw_version=~'%s', duration='%s', formal_test='true'}[%dw]))`
	// Prom query statistic representation for api requests of services.
	APITotalQuery = `max by (namespace, pod, verb) 
	(avg_over_time(apiserver_request_total{}[%s]%s))`
	APITotalTrendQuery = `max without (sw_version) 
	(last_over_time(ranmetrics_apiserver_total_idle_max{cluster='%s', baseline='true',
	sw_version=~'%s', duration='%s', formal_test='true'}[%dw]))`
	// Prom query statistic representation of running pod count in the system.
	ContainerCountQuery = `sum(kube_pod_container_info{pod!~"cnfgotestpriv.*", 
	pod!~"process-exp.*", namespace!~"workload"} * on(namespace, pod) 
	group_left() kube_pod_status_phase{phase="Running"} == 1)`
	ContainerCountBreakdownQuery = `count by (namespace, pod) (kube_pod_container_info{pod!~"cnfgotestpriv.*", 
	pod!~"process-exp.*", namespace!~"workload"} * on(namespace, pod) 
	group_left() kube_pod_status_phase{phase="Running"} == 1)`
	ContainerCountTrendQuery = `max without (sw_version)
	(last_over_time(ranmetrics_container_count_idle_total{cluster='%s', baseline='true',
	sw_version=~'%s', duration='%s', formal_test='true'}[%dw]))`
	ContainerCountBreakdownTrendQuery = `max without (sw_version)
	(last_over_time(ranmetrics_container_count_breakdown_idle_total{cluster='%s', baseline='true',
	sw_version=~'%s', duration='%s', formal_test='true'}[%dw]))`
)
