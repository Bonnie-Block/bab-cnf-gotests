package main

import (
	"context"
	"fmt"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptphelper"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func main() {

	var (
		ptpDaemonPods *corev1.PodList
		err           error
	)

	ptpDaemonPods, err = helper.Apiclient.Pods(parameters.PtpOperatorNamespace).List(context.Background(),
		metav1.ListOptions{
			LabelSelector: parameters.PtpDaemonsetLabelSelector})
	if nil != err {
		fmt.Println(err.Error())
	}

	//testGetPTPMertrics(ptpDaemonPods.Items[0])
	//testRemoveHashSigns(ptpDaemonPods.Items[0])
	//testGetSpecificMetrics(ptpDaemonPods.Items[0])
	testMetricParser(ptpDaemonPods.Items[0])

}

func testGetPTPMertrics(ptpPod corev1.Pod) {
	fmt.Println("****START testGetPTPMertrics TEST****\n\n")
	ptpMetricsBuff, err := ranptphelper.GetPTPMetrics(ptpPod)
	if nil != err {
		fmt.Println(err.Error())
	}
	fmt.Println(ptpMetricsBuff.String())

	fmt.Println("\n\n****END testGetPTPMertrics TEST****\n")
}

func testRemoveHashSigns(ptpPod corev1.Pod) {
	fmt.Println("****START testRemoveHashSigns TEST****\n\n")

	ptpMetricsBuff, err := ranptphelper.GetPTPMetrics(ptpPod)
	if nil != err {
		fmt.Println(err.Error())
	}

	for _, line := range ranptphelper.RemoveHashSigns(ptpMetricsBuff) {
		fmt.Println(line)
	}

	fmt.Println("\n\n****END testRemoveHashSigns TEST****\n")
}

func testGetSpecificMetrics(ptpPod corev1.Pod) {
	fmt.Println("****START testGetSpecificMetrics TEST****\n\n")
	ptpMetricsBuff, err := ranptphelper.GetPTPMetrics(ptpPod)
	if nil != err {
		fmt.Println(err.Error())
	}

	for _, line := range ranptphelper.GetSpecificMetrics(ranptphelper.RemoveHashSigns(ptpMetricsBuff), "openshift_ptp_offset_ns") {
		fmt.Println(line)
	}
	fmt.Println("\n\n****END testGetSpecificMetrics TEST****\n")
}

func testMetricParser(ptpPod corev1.Pod) {
	fmt.Println("****START testMetricParser TEST****\n\n")
	ptpMetricsBuff, err := ranptphelper.GetPTPMetrics(ptpPod)
	if nil != err {
		fmt.Println(err.Error())
	}

	ranptphelper.MetricParser(ranptphelper.RemoveHashSigns(ptpMetricsBuff))

	fmt.Println("\n\n****END testMetricParser TEST****\n")
}
