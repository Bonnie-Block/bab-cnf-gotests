package ranptphelper

import (
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	corev1 "k8s.io/api/core/v1"

	"fmt"
	"strings"
)

// GetProcessPID gets the process id with a given name.
// the function returns an error if any occurred or if the process is not running.
// arguments:		"ptpPod"-			a pod that run the ptp processes.
//
//	"processName"-		the name of a process.
//
// return value:	the pid of the process and an error if any occurred.
func GetProcessPID(ptpPod *corev1.Pod, processName string) (string, error) {
	pidBuff, err := pod.ExecCommand(helper.Apiclient, *ptpPod, []string{"pgrep", processName})
	if nil != err {
		return "", err
	}

	if pidBuff.Len() == 0 {
		return "", fmt.Errorf("process with the name %s is not running", processName)
	}

	return pidBuff.String(), nil
}

// KillPtpProcess kill a process with the given name.
// arguments:		"ptpPod"-			a pod that run the ptp processes.
//
//	"processName"-		the name of a process to be killed.
//
// return value:	an error if any occurred.
func KillPtpProcess(ptpPod *corev1.Pod, processName string) error {
	_, err := pod.ExecCommand(helper.Apiclient, *ptpPod, []string{"pkill", processName})
	if nil != err {
		return err
	}

	return nil
}

// KillProcess kill a process with the given pid.
// arguments:		"ptpPod"-	a pod that run the ptp processes.
//
//	"pid"-		the pid of a process to be killed.
//
// return value:	an error if any occurred.
func KillProcess(ptpPod *corev1.Pod, pid string) error {
	_, err := pod.ExecCommand(helper.Apiclient, *ptpPod, []string{"kill", "-9", pid})
	if nil != err {
		return err
	}

	return nil
}

// GetPhc2sysConfigName gets the right configuration name of the phc2sys process.
// arguments:		"ptpPod"-	a pod that run the ptp processes.
// return value:	a string with the name of the configuration file and an error if any occurred.
func getPhc2sysConfigName(ptpPod *corev1.Pod) (string, error) {
	phc2sysProcessBuff, err := pod.ExecCommand(helper.Apiclient, *ptpPod, []string{"pgrep", "-a", "phc2sys"})
	if nil != err {
		return "", err
	}

	if phc2sysProcessBuff.Len() == 0 {
		return "", fmt.Errorf("phc2sys process not found")
	}

	phc2sysProcess := BytesToStrings(phc2sysProcessBuff)

	return strings.Split(strings.Split(phc2sysProcess[0], "[")[1], "]")[0], nil
}

// GetPTP4lPID gets the wanted ptp4l process if it's the one that's related to the phc2sys or not
// this function is used only for dual nic tests
// arguments:       "ptpPod"-			a pod that run the ptp processes
//                  "relatePHC2SYS"-	TRUE for the ptp process that related to the phc2sys process.
//						FALSE for the onr that isn't.
// return value:	a string with the ptp4l pid. and an error if any occurred.
func GetPTP4lPID(ptpPod *corev1.Pod, relatePHC2SYS bool) (string, error) {
	phc2sysConfigFile, err := getPhc2sysConfigName(ptpPod)
	if nil != err {
		return "", err
	}

	ptp4lProcesses, err := getAllPTPProcess(ptpPod)

	if nil != err {
		return "", err
	}

	if (strings.Contains(ptp4lProcesses[0], phc2sysConfigFile) && relatePHC2SYS) ||
		(!strings.Contains(ptp4lProcesses[0], phc2sysConfigFile) && !relatePHC2SYS) {
		return strings.Split(ptp4lProcesses[0], " ")[0], nil
	}

	return strings.Split(ptp4lProcesses[1], " ")[0], nil
}

// getAllPTPProcess gets all the processes that related to ptp4l.
// arguments:       "ptpPod"-	a pod that has a ptp configuration.
// return value:	an array of strings that holds all ptp4l processes and an error if any occurred.
func getAllPTPProcess(ptpPod *corev1.Pod) ([]string, error) {
	var ptp4lProcesses []string

	allProcsBuff, err := pod.ExecCommand(helper.Apiclient, *ptpPod, []string{"pgrep", "-a", "ptp4l"})

	if nil != err {
		return ptp4lProcesses, err
	}

	allProcsStrs := BytesToStrings(allProcsBuff)
	for _, process := range allProcsStrs {
		if strings.Contains(process, "ptp4l") {
			ptp4lProcesses = append(ptp4lProcesses, process)
		}
	}

	return ptp4lProcesses, nil
}
