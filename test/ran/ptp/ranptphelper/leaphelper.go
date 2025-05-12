package ranptphelper

import (
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptpparameters"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	"log"
	"regexp"

	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// WaitForLeapCMUpdate waits for new announcement with Today's date in leap-configmap.
func WaitForLeapCMUpdate(ptpNodeName string) error {
	interval := 5 * time.Second
	timeout := 10 * time.Minute

	return wait.PollImmediate(interval, timeout, func() (bool, error) {
		todayDate := time.Now().UTC().Format(ranptpparameters.DateFormat)
		newLeapCM, err := helper.Apiclient.ConfigMaps(parameters.PtpOperatorNamespace).Get(context.Background(),
			ranptpparameters.LeapConfigMap, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		if strings.Contains(newLeapCM.Data[ptpNodeName], todayDate) {
			log.Print(newLeapCM.Data[ptpNodeName])

			return true, nil
		}

		return false, nil
	})
}

// RemoveLastLeapAnnouncement deletes the last announcement of a leap event from the leap-configmap.
// The function return values are the patched configmap and an error if occurred.
// Example of how leap-configmap data input looks like:
// data:
//
//	<node-name>: "# Do not edit\n# This file is generated automatically
//	  by linuxptp-daemon\n#$\t3913697179\n#@\t4291747200\n2272060800     10    # 1 Jan
//	  1972\n2287785600     11    # 1 Jul 1972\n2303683200     12    # 1 Jan 1973\n2335219200
//	  \    13    # 1 Jan 1974\n2366755200     14    # 1 Jan 1975\n2398291200     15
//	  \   # 1 Jan 1976\n2429913600     16    # 1 Jan 1977\n2461449600     17    # 1
//	  Jan 1978\n2492985600     18    # 1 Jan 1979\n2524521600     19    # 1 Jan 1980\n2571782400
//	  \    20    # 1 Jul 1981\n2603318400     21    # 1 Jul 1982\n2634854400     22
//	  \   # 1 Jul 1983\n2698012800     23    # 1 Jul 1985\n2776982400     24    # 1
//	  Jan 1988\n2840140800     25    # 1 Jan 1990\n2871676800     26    # 1 Jan 1991\n2918937600
//	  \    27    # 1 Jul 1992\n2950473600     28    # 1 Jul 1993\n2982009600     29
//	  \   # 1 Jul 1994\n3029443200     30    # 1 Jan 1996\n3076704000     31    # 1
//	  Jul 1997\n3124137600     32    # 1 Jan 1999\n3345062400     33    # 1 Jan 2006\n3439756800
//	  \    34    # 1 Jan 2009\n3550089600     35    # 1 Jul 2012\n3644697600     36
//	  \   # 1 Jul 2015\n3692217600     37    # 1 Jan 2017\n\n#h\te65754d4 8f39962b aa854a61
//	  661ef546 d2af0bfa".
//
// In this case the last announcement is "3692217600     37    # 1 Jan 2017".
// The output of the function will be:
// data:
//
//	<node-name>: "# Do not edit\n# This file is generated automatically
//	  by linuxptp-daemon\n#$\t3913697179\n#@\t4291747200\n2272060800     10    # 1 Jan
//	  1972\n2287785600     11    # 1 Jul 1972\n2303683200     12    # 1 Jan 1973\n2335219200
//	  \    13    # 1 Jan 1974\n2366755200     14    # 1 Jan 1975\n2398291200     15
//	  \   # 1 Jan 1976\n2429913600     16    # 1 Jan 1977\n2461449600     17    # 1
//	  Jan 1978\n2492985600     18    # 1 Jan 1979\n2524521600     19    # 1 Jan 1980\n2571782400
//	  \    20    # 1 Jul 1981\n2603318400     21    # 1 Jul 1982\n2634854400     22
//	  \   # 1 Jul 1983\n2698012800     23    # 1 Jul 1985\n2776982400     24    # 1
//	  Jan 1988\n2840140800     25    # 1 Jan 1990\n2871676800     26    # 1 Jan 1991\n2918937600
//	  \    27    # 1 Jul 1992\n2950473600     28    # 1 Jul 1993\n2982009600     29
//	  \   # 1 Jul 1994\n3029443200     30    # 1 Jan 1996\n3076704000     31    # 1
//	  Jul 1997\n3124137600     32    # 1 Jan 1999\n3345062400     33    # 1 Jan 2006\n3439756800
//	  \    34    # 1 Jan 2009\n3550089600     35    # 1 Jul 2012\n3644697600     36
//	  \   # 1 Jul 2015\n\n#h\te65754d4 8f39962b aa854a61 661ef546 d2af0bfa".
func RemoveLastLeapAnnouncement(leapCM *v1.ConfigMap, nodeName string) (*v1.ConfigMap,
	error) {
	newCMData := editLeapCMData(leapCM.Data[nodeName])

	newDataMap := make(map[string]string)
	newDataMap[nodeName] = newCMData

	log.Printf("current data:\n%s\n", leapCM.Data[nodeName])

	patchedCM, err := patchNewDataCM(newDataMap)
	if err != nil {
		return nil, err
	}

	log.Printf("data after removing last announcement:\n%s\n", patchedCM.Data[nodeName])

	leapCMLastAnnouncement, err := GetLastAnnouncement(leapCM, nodeName)
	if err != nil {
		return nil, err
	}

	patchedCMLastAnnouncement, err := GetLastAnnouncement(patchedCM, nodeName)
	if err != nil {
		return nil, err
	}

	if leapCMLastAnnouncement == patchedCMLastAnnouncement {
		return nil, fmt.Errorf("remove last announecemet failed")
	}

	return patchedCM, nil
}

// ClearLeapCmData removes all leap-config map data and patches it to the leap-configmap.
func ClearLeapCmData(leapCM *v1.ConfigMap, nodeName string) (*v1.ConfigMap, error) {

	log.Println("removing data from leap-configmap")
	newDataMap := make(map[string]string)

	log.Println("patching leap-configmap with empty data")
	patchedCM, err := patchNewDataCM(newDataMap)
	if err != nil {
		return nil, err
	}

	if len(patchedCM.Data) != 0 {
		return nil, fmt.Errorf("remove data failed")
	}

	return patchedCM, nil
}

// GetLastAnnouncement returns the last leap event announcement from a leap-configmap Data.
// An example of returned value: "3550089600     35    # 1 Jul 2012".
func GetLastAnnouncement(leapCM *v1.ConfigMap, nodeNane string) (string, error) {
	cmData := leapCM.Data[nodeNane]
	if len(cmData) == 0 {
		return cmData, nil
	}

	r := regexp.MustCompile(`\n(\d+\s+\d+\s+#\s\d+\s[a-zA-Z]+\s\d{4})\n\n`)
	lastAnnouncementSlice := r.FindStringSubmatch(cmData)

	if len(lastAnnouncementSlice) < 2 {
		return "", fmt.Errorf("error finding the last announcement")
	}

	return lastAnnouncementSlice[1], nil
}

// patchNewDataCM patches the new given value of Data in configmap and returns the patched config map and an error if
// occurred.
func patchNewDataCM(newValue map[string]string) (*v1.ConfigMap, error) {

	patch := []ranptpparameters.PatchInterfaceValue{{
		Op:    "replace",
		Path:  "/data",
		Value: newValue,
	}}

	newPatchPtpBytes, err := json.Marshal(patch)
	if err != nil {
		return nil, err
	}

	updatedLeapCM, err := helper.Apiclient.ConfigMaps(parameters.PtpOperatorNamespace).Patch(context.Background(),
		ranptpparameters.LeapConfigMap, types.JSONPatchType, newPatchPtpBytes, metav1.PatchOptions{})
	if err != nil {
		return nil, err
	}

	return updatedLeapCM, nil
}

// editLeapCMData removes the last leap event announcement of leap-configmap Data.
func editLeapCMData(leapCMData string) string {
	originalLeapCMDataSlice := strings.Split(leapCMData, "\n")

	oldAnnouncements := originalLeapCMDataSlice[0 : len(originalLeapCMDataSlice)-3]
	endOfData := originalLeapCMDataSlice[len(originalLeapCMDataSlice)-2:]

	newLeapCMDataSlice := make([]string, 0, len(oldAnnouncements)+len(endOfData))
	newLeapCMDataSlice = append(newLeapCMDataSlice, oldAnnouncements...)
	newLeapCMDataSlice = append(newLeapCMDataSlice, endOfData...)

	return strings.Join(newLeapCMDataSlice, "\n")
}
