package k8sreporter

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/onsi/ginkgo/v2/config"
	"github.com/onsi/ginkgo/v2/types"

	"github.com/kennygrant/sanitize"
	testclient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// FilterByNamespace is a filter function to choose what resources to filter by a given namespace.
type FilterByNamespace func(string) bool

// AddToScheme is a function for extend the reporter scheme and the CRs we are able to dump.
type AddToScheme func(*runtime.Scheme)

// KubernetesReporter is a Ginkgo reporter that dumps info
// about configured kubernetes objects.
type KubernetesReporter struct {
	sync.Mutex
	clients         *testclient.ClientSet
	reportPath      string
	filterResources FilterByNamespace
	crs             []CRData
}

// CRData represents a cr to dump.
type CRData struct {
	Cr        runtimeclient.ObjectList
	Namespace *string
}

// New returns a new Kubernetes reporter from the given configuration.
func New(
	kubeconfig string,
	addToScheme AddToScheme,
	resourcesToLog FilterByNamespace,
	reportPath string, crs ...CRData) (*KubernetesReporter, error) {
	crScheme := runtime.NewScheme()

	if err := clientgoscheme.AddToScheme(crScheme); err != nil {
		return nil, err
	}

	addToScheme(crScheme)

	clients := testclient.New(kubeconfig)

	crsToDump := []CRData{}
	if crs != nil {
		crsToDump = crs
	}

	return &KubernetesReporter{
		clients:         clients,
		reportPath:      reportPath,
		filterResources: resourcesToLog,
		crs:             crsToDump,
	}, nil
}

// SpecSuiteWillBegin is the ginkgo callback on beginning of suite.
func (r *KubernetesReporter) SpecSuiteWillBegin(config config.GinkgoConfigType, summary *types.SuiteSummary) {

}

// BeforeSuiteDidRun is the ginkgo callback on running of a suite.
func (r *KubernetesReporter) BeforeSuiteDidRun(setupSummary *types.SetupSummary) {
	r.Cleanup()
}

// SpecWillRun is the ginkgo callback on running of a spec.
func (r *KubernetesReporter) SpecWillRun(specSummary *types.SpecSummary) {
}

// SpecDidComplete is the ginkgo callback on finishing a spec.
func (r *KubernetesReporter) SpecDidComplete(specSummary *types.SpecSummary) {
	r.Lock()
	defer r.Unlock()

	if !specSummary.HasFailureState() {
		return
	}

	logFile, err := logFileFor(r.reportPath, "all", "")

	if err != nil {
		return
	}

	defer func() {
		_ = logFile.Close()
	}()

	_, _ = fmt.Fprintln(logFile, "Starting dump for failed spec", specSummary.ComponentTexts)
	dirName := sanitize.BaseName(strings.Join(specSummary.ComponentTexts, ""))
	dirName = strings.Replace(dirName, "Top-Level", "", 1)

	r.Dump(specSummary.RunTime, dirName)

	_, _ = fmt.Fprintln(logFile, "Finished dump for failed spec")
}

// Dump dumps the relevant crs + pod logs.
func (r *KubernetesReporter) Dump(duration time.Duration, dirName string) {
	since := time.Now().Add(-duration).Add(-5 * time.Second)

	if _, err := os.Stat(path.Join(r.reportPath, dirName)); os.IsNotExist(err) {
		err := os.MkdirAll(path.Join(r.reportPath, dirName), 0755)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to create test dir: %v\n", err)

			return
		}
	}

	r.logNodes(dirName)
	r.logLogs(since, dirName)
	r.logPods(dirName)

	for _, cr := range r.crs {
		r.logCustomCR(cr.Cr, cr.Namespace, dirName)
	}
}

// Cleanup cleans up the current content of the artifactsDir.
func (r *KubernetesReporter) Cleanup() {
}

func (r *KubernetesReporter) logPods(dirName string) {
	pods, err := r.clients.Pods(v1.NamespaceAll).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to fetch pods: %v\n", err)

		return
	}

	for _, pod := range pods.Items {
		if r.filterResources(pod.Namespace) {
			continue
		}

		logFile, err := logFileFor(r.reportPath, dirName, pod.Namespace+"-pods_specs")

		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to open pods_specs file: %v\n", dirName)

			return
		}

		defer func() {
			_ = logFile.Close()
		}()

		_, _ = fmt.Fprintf(logFile, "-----------------------------------\n")
		jByte, err := json.MarshalIndent(pod, "", "    ")

		if err != nil {
			fmt.Println("Failed to marshal pods", err)

			return
		}

		_, _ = fmt.Fprintln(logFile, string(jByte))
	}
}

func (r *KubernetesReporter) logNodes(dirName string) {
	logFile, err := logFileFor(r.reportPath, dirName, "nodes")
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to open nodes file: %v\n", dirName)

		return
	}

	defer func() {
		_ = logFile.Close()
	}()

	_, _ = fmt.Fprintf(logFile, "-----------------------------------\n")

	nodes, err := r.clients.Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to fetch nodes: %v\n", err)

		return
	}

	jBytes, err := json.MarshalIndent(nodes, "", "    ")
	if err != nil {
		fmt.Println("Failed to marshal nodes")

		return
	}

	_, _ = fmt.Fprintln(logFile, string(jBytes))
}

func (r *KubernetesReporter) logLogs(since time.Time, dirName string) {
	pods, err := r.clients.Pods(v1.NamespaceAll).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to fetch pods: %v\n", err)

		return
	}

	for _, pod := range pods.Items {
		if r.filterResources(pod.Namespace) {
			continue
		}

		logFile, err := logFileFor(r.reportPath, dirName, pod.Namespace+"-pods_logs")

		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to open pods_logs file: %v\n", dirName)

			return
		}

		defer func() {
			_ = logFile.Close()
		}()

		containersToLog := make([]v1.Container, 0)
		containersToLog = append(containersToLog, pod.Spec.Containers...)
		containersToLog = append(containersToLog, pod.Spec.InitContainers...)

		for _, container := range containersToLog {
			logStart := metav1.NewTime(since)
			logs, err := r.clients.Pods(pod.Namespace).GetLogs(
				pod.Name,
				&v1.PodLogOptions{
					Container: container.Name,
					SinceTime: &logStart,
				},
			).DoRaw(context.Background())

			if err == nil {
				_, _ = fmt.Fprintf(logFile, "-----------------------------------\n")
				_, _ = fmt.Fprintf(logFile, "Dumping logs for pod %s-%s-%s\n", pod.Namespace, pod.Name, container.Name)
				_, _ = fmt.Fprintln(logFile, string(logs))
			}
		}
	}
}

func (r *KubernetesReporter) logCustomCR(customResource runtimeclient.ObjectList, namespace *string, dirName string) {
	logFile, err := logFileFor(r.reportPath, dirName, "crs")
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to open crs file: %v\n", dirName)

		return
	}

	defer func() {
		_ = logFile.Close()
	}()

	_, _ = fmt.Fprintf(logFile, "-----------------------------------\n")

	if namespace != nil {
		_, _ = fmt.Fprintf(logFile, "Dumping %T in namespace %s\n", customResource, *namespace)
	} else {
		_, _ = fmt.Fprintf(logFile, "Dumping %T\n", customResource)
	}

	options := []runtimeclient.ListOption{}
	if namespace != nil {
		options = append(options, runtimeclient.InNamespace(*namespace))
	}

	err = r.clients.List(context.Background(),
		customResource,
		options...)

	if err != nil {
		// this can be expected if we are reporting a feature we did not install the operator for
		_, _ = fmt.Fprintf(logFile, "Failed to fetch %T: %v\n", customResource, err)

		return
	}

	jBytes, err := json.MarshalIndent(customResource, "", "    ")
	if err != nil {
		_, _ = fmt.Fprintf(logFile, "Failed to marshal %T\n", customResource)

		return
	}

	_, _ = fmt.Fprintln(logFile, string(jBytes))
}

// AfterSuiteDidRun is the ginkgo callback after suite run.
func (r *KubernetesReporter) AfterSuiteDidRun(setupSummary *types.SetupSummary) {

}

// SpecSuiteDidEnd is the ginkgo callback after end of suite.
func (r *KubernetesReporter) SpecSuiteDidEnd(summary *types.SuiteSummary) {

}

func logFileFor(dirName string, testName string, kind string) (*os.File, error) {
	logFPath := path.Join(dirName, testName, kind) + ".log"
	openedFile, err := os.OpenFile(logFPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)

	if err != nil {
		return nil, err
	}

	return openedFile, nil
}
