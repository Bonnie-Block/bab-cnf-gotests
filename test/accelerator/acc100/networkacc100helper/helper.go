package helper

import (
	"fmt"
	"time"

	. "github.com/onsi/gomega"

	fecv2 "github.com/smart-edge-open/openshift-operator/sriov-fec/api/v2"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/accelerator/acc100/parameters"
	helper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/accelerator/networkacceleratorhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetSriovFecNodeForAcc100 retrieves SriovFecNodeConfig
func GetSriovFecNodeForAcc100(cs *client.ClientSet) (*fecv2.SriovFecNodeConfig, *fecv2.SriovAccelerator, error) {
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
func GetSriovFecAcc100ClusterConfigDefinition(cs *client.ClientSet, isSingleNode bool) *fecv2.SriovFecClusterConfig {
	var (
		err                error
		sriovFecNodeConfig *fecv2.SriovFecNodeConfig
		accelerator        *fecv2.SriovAccelerator
		vf                 = 2
	)

	Eventually(func() error {
		sriovFecNodeConfig, accelerator, err = GetSriovFecNodeForAcc100(cs)
		return err
	}, 2*time.Minute, 1*time.Second).ShouldNot(HaveOccurred(), "there are no available SriovAccelerators")

	queueGroupConfig := fecv2.QueueGroupConfig{
		AqDepthLog2:     4,
		NumAqsPerGroups: 16,
		NumQueueGroups:  2,
	}

	sriovFecClusterConfig := &fecv2.SriovFecClusterConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "config", Namespace: parameters.OperatorNamespace},
		Spec: fecv2.SriovFecClusterConfigSpec{
			Priority: 1,
			NodeSelector: map[string]string{
				"kubernetes.io/hostname": sriovFecNodeConfig.Name,
			},
			AcceleratorSelector: fecv2.AcceleratorSelector{
				PCIAddress: accelerator.PCIAddress,
			},
			PhysicalFunction: fecv2.PhysicalFunctionConfig{
				PFDriver: "pci-pf-stub",
				VFAmount: vf,
				VFDriver: "vfio-pci",
				BBDevConfig: fecv2.BBDevConfig{
					ACC100: &fecv2.ACC100BBDevConfig{
						Downlink4G:   queueGroupConfig,
						Downlink5G:   queueGroupConfig,
						Uplink4G:     queueGroupConfig,
						Uplink5G:     queueGroupConfig,
						PFMode:       false,
						MaxQueueSize: 1024,
						NumVfBundles: vf,
					},
				},
			},
		}}
	sriovFecClusterConfig.Spec.DrainSkip = isSingleNode
	return sriovFecClusterConfig
}
