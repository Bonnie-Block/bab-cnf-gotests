package ranptphelper

import (
	"bytes"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

// GetProcessPID gets the process id with a given name.
// the function returns an error if any occurred or if the process is not running.
// Arguments:
// "ptpPod"-		a pod that run the ptp processes.
// "processName"-	the name of a process.
// return value:	the pid of the process and an error if any occurred.
func GetProcessPID(ptpPod *corev1.Pod, processName string) (string, error) {
	pidBuff, err := getProcessInfo(ptpPod, "pgrep "+processName)
	if err != nil {
		return "", err
	}

	if pidBuff.Len() == 0 {
		return "", fmt.Errorf("process with the name %s is not running", processName)
	}

	pid := string(bytes.TrimRight(pidBuff.Bytes(), "\r\n"))

	return pid, nil
}

// WaitForProcess waits for given process to appear and returns the process id.
func WaitForProcess(ptpPod *corev1.Pod, processName string) (string, error) {
	var (
		pid string
		err error
	)

	waitErr := wait.PollImmediate(3*time.Second, 1*time.Minute, func() (bool, error) {
		pid, err = GetProcessPID(ptpPod, processName)

		return err == nil, nil
	})

	return pid, waitErr
}

// KillPtpProcess kill a process with the given name.
// Arguments:
// "ptpPod"-		a pod that run the ptp processes.
// "processName"-	the name of a process to be killed.
// return value:	an error if any occurred.
func KillPtpProcess(ptpPod *corev1.Pod, processName string) error {
	_, err := pod.ExecCommand(helper.Apiclient, *ptpPod, []string{"pkill", processName})
	if nil != err {
		return err
	}

	return nil
}

// KillProcess kill a process with the given pid.
// Arguments:
// "ptpPod"-		a pod that run the ptp processes.
// "pid"	-		the pid of a process to be killed.
// return value:	an error if any occurred.
func KillProcess(ptpPod *corev1.Pod, pid string) error {
	_, err := pod.ExecCommand(helper.Apiclient, *ptpPod, []string{"kill", "-9", pid})
	if nil != err {
		return err
	}

	return nil
}

func getProcessInfo(ptpPod *corev1.Pod, command string) (bytes.Buffer, error) {
	var (
		cmdOutput bytes.Buffer
		errActual error
	)

	timeoutErr := wait.PollImmediate(3*time.Second, 30*time.Second, func() (done bool, err error) {
		cmdOutput, errActual = pod.ExecCommand(helper.Apiclient, *ptpPod, []string{"bash", "-c", command + "; sleep 0.1"},
			parameters.PtpContainerName)
		if errActual != nil {
			return false, nil
		}

		if cmdOutput.Len() == 0 {
			errActual = fmt.Errorf("process not found")

			return false, nil
		}

		return true, nil
	})

	if timeoutErr != nil {
		return cmdOutput, errActual
	}

	return cmdOutput, nil
}

// GetPtp4lPids gets ptp4l processes with given relationship with the phc2sys.
// Note that if only 1 ptp profile is configured, then no process will be returned when relateToProcess=false.
func GetPtp4lPids(ptpPod *corev1.Pod, processName string, relateToProcess bool) ([]string, error) {
	processConfigFile, err := getProcessRelatedPtp4lConfig(ptpPod, processName)
	if nil != err {
		return nil, err
	}

	ptp4lProcesses, err := getAllPtp4lProcesses(ptpPod)
	if nil != err {
		return nil, err
	}

	var processes []string

	for _, ptp4lProcess := range ptp4lProcesses {
		if strings.Contains(ptp4lProcess, processConfigFile) == relateToProcess {
			processes = append(processes, strings.Split(ptp4lProcess, " ")[0])
		}
	}

	return processes, nil
}

// getAllPtp4lProcesses gets all the processes that related to ptp4l.
// Arguments:
// "ptpPod"-		a pod that has a ptp configuration.
// return value:	an array of strings that holds all ptp4l processes and an error if any occurred.
func getAllPtp4lProcesses(ptpPod *corev1.Pod) ([]string, error) {
	var ptp4lProcesses []string

	allProcsBuff, err := getProcessInfo(ptpPod, "pgrep -a ptp4l")
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

// GetPhc2sysInterface returns interface configured for phc2sys from ptp config file under /var/run.
func GetPhc2sysInterface(ptpPod *corev1.Pod) (string, error) {
	configName, err := getProcessRelatedPtp4lConfig(ptpPod, "phc2sys")
	if err != nil {
		return "", err
	}

	cmdOc := fmt.Sprintf("cat /var/run/%s | grep '^\\[en.*\\]' | tr -d '[]'", configName)
	buff, err := pod.ExecCommand(helper.Apiclient, *ptpPod, []string{"bash", "-c", cmdOc})

	if err != nil {
		return "", err
	}

	// If BC config, then rerun cmd to find slave in BC config
	if len(BytesToStrings(buff)) != 1 {
		log.Println("More than 1 interface found in ptp4lconfig - switch to BC parser")

		cmdBC := fmt.Sprintf("cat /var/run/%s | grep -A 1 '^\\[en.*\\]' | grep -B 1 '^masterOnly.*0' | "+
			"grep '^\\[en' | tr -d '[]'", configName)
		buff, err = pod.ExecCommand(helper.Apiclient, *ptpPod, []string{"bash", "-c", cmdBC})

		if err != nil {
			return "", err
		}
	}

	return buff.String(), nil
}

// getProcessRelatedPtp4lConfig gets ptp4l config that is related to a given process.
// Arguments:
// "ptpPod"-		a pod that run the ptp processes.
// "processName"	the name of the related process (e.g "phc2sys" of "ts2phc"...).
// return value:	a string with the name of the configuration file and an error if any occurred.
func getProcessRelatedPtp4lConfig(ptpPod *corev1.Pod, processName string) (string, error) {
	ptp4lConfigRegex := regexp.MustCompile(`ptp4l.[\d]+.config`)

	processBuff, err := getProcessInfo(ptpPod, "pgrep -a "+processName+" | grep /var/run")
	if err != nil {
		return "", err
	}

	process := BytesToStrings(processBuff)[0]
	// handles  process output like this:
	// "/usr/sbin/phc2sys -a -r -n 24 -m -u 1 -z /var/run/ptp4l.0.socket -t [ptp4l.0.config]"
	if strings.Contains(process, "[ptp4l") {
		return ptp4lConfigRegex.FindAllString(process, 1)[0], nil
	}

	// handles process output like this:
	// "/usr/sbin/phc2sys -f /var/run/phc2sys.1.config  -s enp49s0f0 -w -r -m -n 24 -N 8 -R 16"
	if strings.Contains(process, "-f /var/run/"+processName) {
		processConfigRegex := regexp.MustCompile(fmt.Sprintf("/var/run/%s.[\\d]+.config", processName))

		processConfig := processConfigRegex.FindAllString(process, 1)[0]

		// Expect output like this: "uds_address [ptp4l.0.socket]"
		ptp4lConfigBuff, err := getProcessInfo(ptpPod,
			fmt.Sprintf("cat %s | grep --color=no uds_address", processConfig))

		if err != nil {
			return "", err
		}

		return strings.TrimSuffix(
			strings.TrimSpace(strings.Split(ptp4lConfigBuff.String(), " ")[1]), "socket") + "config", nil
	}

	return "", fmt.Errorf("output unrecognized: %v", process)
}
