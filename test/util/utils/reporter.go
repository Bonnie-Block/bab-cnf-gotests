package utils

import (
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"strings"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"

	"github.com/onsi/ginkgo/v2/types"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/k8sreporter"
	"k8s.io/apimachinery/pkg/runtime"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	mcfgv1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	ptpv1 "github.com/openshift/ptp-operator/api/v1"
)

// NewReporter creates a specific reporter for CNF tests.
func NewReporter(
	reportPath string,
	namespacesToDump map[string]string,
	crds []k8sreporter.CRData) (*k8sreporter.KubernetesReporter, error) {
	addToScheme := func(scheme *runtime.Scheme) {
		err := ptpv1.AddToScheme(scheme)
		if err != nil {
			panic(err)
		}

		err = mcfgv1.AddToScheme(scheme)

		if err != nil {
			panic(err)
		}

		err = sriovv1.AddToScheme(scheme)

		if err != nil {
			panic(err)
		}
	}

	skipByNamespace := func(ns string) bool {
		_, found := namespacesToDump[ns]

		return !found
	}

	if _, err := os.Stat(reportPath); os.IsNotExist(err) {
		err := os.MkdirAll(reportPath, 0755)
		if err != nil {
			panic(fmt.Errorf("can not create report dir due to %w", err))
		}
	}

	res, err := k8sreporter.New("", addToScheme, skipByNamespace, reportPath, crds...)

	if err != nil {
		return nil, err
	}

	return res, nil
}

func ReportIfFailed(report types.SpecReport, testSuite string, nSpaces map[string]string, cRDs []k8sreporter.CRData) {
	if report.State == types.SpecStateAborted || report.State == types.SpecStateFailed ||
		report.State == types.SpecStateInterrupted {
		dumpFile := helper.Config.GetDumpFailedTestReportLocation(testSuite)
		if dumpFile != "" {
			reporter, err := NewReporter(dumpFile, nSpaces, cRDs)

			if err != nil {
				log.Fatalf("Failed to create log reporter %s", err)
			}

			tcReportFolderName := strings.ReplaceAll(report.FullText(), " ", "_")
			reporter.Dump(report.RunTime, tcReportFolderName)

			_, podExecLogsFName := path.Split(config.PathToPodExecLogs)
			err = moveFile(
				config.PathToPodExecLogs, path.Join(reporter.ReportPath, tcReportFolderName, podExecLogsFName))

			if err != nil {
				log.Fatalf("Failed to move pod exec logs %s to report folder: %s", config.PathToPodExecLogs, err)
			}
		}
	}

	removeFile(config.PathToPodExecLogs)
}

func moveFile(sourcePath, destPath string) error {
	inputFile, err := os.Open(sourcePath)

	if err != nil {
		return fmt.Errorf("couldn't open source file: %w", err)
	}

	defer func() {
		_ = inputFile.Close()
	}()

	outputFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("couldn't open dest file: %w", err)
	}

	defer func() {
		_ = outputFile.Close()
	}()

	_, err = io.Copy(outputFile, inputFile)

	if err != nil {
		return fmt.Errorf("writing to output file failed: %w", err)
	}

	return nil
}

func removeFile(fPath string) {
	if _, err := os.Stat(fPath); err == nil {
		err := os.Remove(fPath)
		if err != nil {
			log.Fatalf("Failed to remove pod exec logs from %s: %s", fPath, err)
		}
	}
}
