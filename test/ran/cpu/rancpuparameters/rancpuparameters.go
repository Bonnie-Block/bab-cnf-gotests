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
	RanCPUMetricOsDaemon  = "ranmetrics_cpu_os_daemon"
	RanCPUMetricInfraPods = "ranmetrics_cpu_infra_pods"
	RanCPUMetricTotal     = "ranmetrics_cpu_total"
	RanAPIServerRate      = "ranmetrics_apiserver_rate"
	RanAPIServerTotal     = "ranmetrics_apiserver_total"
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
	OsTrendQuery = `avg by (groupname, pod)
	(max_over_time(ranmetrics_cpu_os_daemon_steadyworkload_avg{cluster='%s',
	baseline='true', sw_version=~'%s', duration='%s'}[%dw]))`
	PodTrendQuery = `avg by (namespace, pod)
	(max_over_time(ranmetrics_cpu_infra_pods_steadyworkload_avg{cluster='%s',
	baseline='true', sw_version=~'%s', duration='%s'}[%dw]))`
	// Prom query statistic representation for api requests of services.
	APITotalQuery = `avg by (namespace, pod, verb) 
	(max_over_time(apiserver_request_total{}[%s]%s))`
	APITotalTrendQuery = `avg by (namespace, pod, verb) 
	(max_over_time(ranmetrics_apiserver_total_idle_max{cluster='%s',baseline='true'}[%dw]))`
)
