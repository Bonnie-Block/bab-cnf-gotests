package ranptphelper

import (
	"bytes"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

// GetProcessPID gets the process id with a given name.
// the function returns an error if any occurred or if the process is not running.
// arguments:		"ptpPod"-			a pod that run the ptp processes.
//
//	"processName"-		the name of a process.
//
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

// getPhc2sysRelatedPtp4lConfig gets ptp4l config that is related to phc2sys process.
// arguments:		"ptpPod"-	a pod that run the ptp processes.
// return value:	a string with the name of the configuration file and an error if any occurred.
func getPhc2sysRelatedPtp4lConfig(ptpPod *corev1.Pod) (string, error) {
	ptp4lConfigRegex := regexp.MustCompile(`ptp4l.[\d]+.config`)

	phc2sysProcessBuff, err := getProcessInfo(ptpPod, "pgrep -a phc2sys | grep /var/run")
	if err != nil {
		return "", err
	}

	phc2sysProcess := BytesToStrings(phc2sysProcessBuff)[0]

	// handles phc2sys process output like this:
	// "/usr/sbin/phc2sys -a -r -n 24 -m -u 1 -z /var/run/ptp4l.0.socket -t [ptp4l.0.config]"
	if strings.Contains(phc2sysProcess, "[ptp4l") {
		return ptp4lConfigRegex.FindAllString(phc2sysProcess, 1)[0], nil
	}

	// handles phc2sys process output like this:
	// "/usr/sbin/phc2sys -f /var/run/phc2sys.1.config  -s enp49s0f0 -w -r -m -n 24 -N 8 -R 16"
	if strings.Contains(phc2sysProcess, "-f /var/run/phc2sys") {
		phc2sysConfigRegex := regexp.MustCompile(`/var/run/phc2sys.[\d]+.config`)

		phc2sysConfig := phc2sysConfigRegex.FindAllString(phc2sysProcess, 1)[0]
		// Expect output like this: "message_tag [ptp4l.0.config]"
		ptp4lConfgBuff, err := getProcessInfo(ptpPod,
			fmt.Sprintf("cat %s | grep --color=no '\\[ptp4l'", phc2sysConfig))

		if err != nil {
			return "", err
		}

		return ptp4lConfigRegex.FindAllString(BytesToStrings(ptp4lConfgBuff)[0], 1)[0], nil
	}

	return "", fmt.Errorf("output unrecognized: %v", phc2sysProcess)
}

func getProcessInfo(ptpPod *corev1.Pod, command string) (bytes.Buffer, error) {
	var (
		cmdOutput bytes.Buffer
		errActual error
	)

	timeoutErr := wait.PollImmediate(3*time.Second, 30*time.Second, func() (done bool, err error) {
		cmdOutput, errActual = pod.ExecCommand(helper.Apiclient, *ptpPod, []string{"bash", "-c", command})
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
// Note that if only 1 ptp profile is configured, then no process will be returned when relatePhc2sys=false.
func GetPtp4lPids(ptpPod *corev1.Pod, relatePhc2sys bool) ([]string, error) {
	phc2sysConfigFile, err := getPhc2sysRelatedPtp4lConfig(ptpPod)
	if nil != err {
		return nil, err
	}

	ptp4lProcesses, err := getAllPtp4lProcesses(ptpPod)
	if nil != err {
		return nil, err
	}

	var processes []string

	for _, ptp4lProcess := range ptp4lProcesses {
		if strings.Contains(ptp4lProcess, phc2sysConfigFile) == relatePhc2sys {
			processes = append(processes, strings.Split(ptp4lProcess, " ")[0])
		}
	}

	return processes, nil
}

// getAllPtp4lProcesses gets all the processes that related to ptp4l.
// arguments:       "ptpPod"-	a pod that has a ptp configuration.
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
	configName, err := getPhc2sysRelatedPtp4lConfig(ptpPod)
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
