package ranptphelper

import (
	"fmt"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	corev1 "k8s.io/api/core/v1"

	"strings"
)

// GetProcessPID gets a process name, 'processName', and a pod, 'ptpPod',
// and return the process id in that pod, if the process id running.
// the function returns an error if any occurred or if the process is not running.
func GetProcessPID(processName string, ptpPod *corev1.Pod) (string, error) {
	pidBuff, err := pod.ExecCommand(helper.Apiclient, *ptpPod, []string{"pgrep", processName})
	if nil != err {
		return "", err
	}

	if pidBuff.Len() == 0 {
		return "", fmt.Errorf("process with the name %s is not running", processName)
	}

	return pidBuff.String(), nil
}

// KillPtpProcess gets a process name, 'processName', and a pod, 'ptpPod', and kill that process.
// an error returns if any occurred.
func KillPtpProcess(processName string, ptpPod *corev1.Pod) error {
	_, err := pod.ExecCommand(helper.Apiclient, *ptpPod, []string{"pkill", processName})
	if nil != err {
		return err
	}

	return nil
}

// GetPhc2sysConfigName gets the right configuration name of the phc2sys process.
// arguments:       ""  -.
// return value:.
func GetPhc2sysConfigName(ptpPod *corev1.Pod) (string, error) {
	phc2sysProcess, err := GetProcessPID("phc2sys", ptpPod)
	if nil != err {
		return "", err
	}

	return strings.Split(strings.Split(phc2sysProcess, "[")[1], "]")[0], nil
}
