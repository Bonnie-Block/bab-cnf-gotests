package helper

import (
	"context"
	"fmt"
	"time"

	"github.com/openshift-kni/performance-addon-operators/api/v2"

	"k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CreatePerformanceProfile creates performance profile
func CreatePerformanceProfile(performanceProfileName string, mcpPoolName string) error {
	isolatedCPUSet := v2.CPUSet("8-15")
	reservedCPUSet := v2.CPUSet("0-7")
	hugepageSize := v2.HugePageSize("1G")
	performanceProfile := &v2.PerformanceProfile{
		ObjectMeta: v1.ObjectMeta{
			Name: performanceProfileName,
		},
		Spec: v2.PerformanceProfileSpec{
			CPU: &v2.CPU{
				Isolated: &isolatedCPUSet,
				Reserved: &reservedCPUSet,
			},
			HugePages: &v2.HugePages{
				DefaultHugePagesSize: &hugepageSize,
				Pages: []v2.HugePage{
					{
						Count: 10,
						Size:  hugepageSize,
					},
				},
			},
			NodeSelector: map[string]string{
				fmt.Sprintf("%s", mcpPoolName): "",
			},
		},
	}
	return Apiclient.Client.Create(context.TODO(), performanceProfile)
}

// CleanAllPerformanceProfile removes all PerformanceProfile from cluster
func CleanAllPerformanceProfile(cnfNodeLabel string, snoTimeoutMultiplier time.Duration) error {
	performanceProfileList := &v2.PerformanceProfileList{}
	err := Apiclient.Client.List(context.TODO(), performanceProfileList)
	if err != nil {
		return err
	}
	if len(performanceProfileList.Items) > 0 {
		for _, performanceProfile := range performanceProfileList.Items {
			err := Apiclient.Client.Delete(
				context.TODO(),
				&performanceProfile)
			if err != nil {
				return err
			}
		}
		err = WaitForClusterToBeStable(cnfNodeLabel, snoTimeoutMultiplier)
		if err != nil {
			return err
		}

	}
	return nil
}
