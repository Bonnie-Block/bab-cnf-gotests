package helper

import (
	"fmt"
	"time"

	. "github.com/onsi/gomega"

	fecv1 "github.com/open-ness/openshift-operator/sriov-fec/api/v1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/accelerator/acc100/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/accelerator/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetSriovFecNodeForAcc100 retrieves SriovFecNodeConfig
func GetSriovFecNodeForAcc100(cs *client.ClientSet) (*fecv1.SriovFecNodeConfig, *fecv1.SriovAccelerator, error) {
	sriovFecNodeConfigList, err := helper.GetSriovFecNodeConfigList(cs)
	if err != nil {
		return nil, nil, err
	}
	for _, sriovFecNodeConfig := range sriovFecNodeConfigList.Items {
		for _, accelerators := range sriovFecNodeConfig.Status.Inventory.SriovAccelerators {
			if accelerators.DeviceID == parameters.Acc100DeviceID {
				return &sriovFecNodeConfig, &accelerators, nil
			}
		}
	}
	return nil, nil, fmt.Errorf("SriovFecNodeConfigList %v doesn`t have sriovfecnodeconfig with configured nic", sriovFecNodeConfigList)
}

// GetSriovFecAcc100ClusterConfigDefinition retrieves SriovFecClusterConfig definition
func GetSriovFecAcc100ClusterConfigDefinition(cs *client.ClientSet, isDefaultConfig bool, isSingleNode bool) *fecv1.SriovFecClusterConfig {
	var err error
	var sriovFecNodeConfig *fecv1.SriovFecNodeConfig
	var accelerator *fecv1.SriovAccelerator

	vf := 0
	if !isDefaultConfig {
		vf = 2
	}

	Eventually(func() error {
		sriovFecNodeConfig, accelerator, err = GetSriovFecNodeForAcc100(cs)
		return err
	}, 2*time.Minute, 1*time.Second).ShouldNot(HaveOccurred(), "there are no available SriovAccelerators")

	queueGroupConfig := fecv1.QueueGroupConfig{
		AqDepthLog2:     4,
		NumAqsPerGroups: 16,
		NumQueueGroups:  2,
	}

	sriovFecClusterConfig := &fecv1.SriovFecClusterConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "config", Namespace: parameters.OperatorNamespace},
		Spec: fecv1.SriovFecClusterConfigSpec{
			Nodes: []fecv1.NodeConfig{
				{
					NodeName: sriovFecNodeConfig.Name,
					PhysicalFunctions: []fecv1.PhysicalFunctionConfig{
						{
							PCIAddress: accelerator.PCIAddress,
							PFDriver:   "pci-pf-stub",
							VFAmount:   vf,
							VFDriver:   "vfio-pci",
							BBDevConfig: fecv1.BBDevConfig{
								ACC100: &fecv1.ACC100BBDevConfig{
									Downlink4G:   queueGroupConfig,
									Downlink5G:   queueGroupConfig,
									Uplink4G:     queueGroupConfig,
									Uplink5G:     queueGroupConfig,
									PFMode:       false,
									MaxQueueSize: 1024,
									NumVfBundles: 16,
								},
							},
						},
					},
				},
			},
		}}
	sriovFecClusterConfig.Spec.DrainSkip = isSingleNode
	return sriovFecClusterConfig
}
