package helper

import (
	"context"
	"fmt"
	"time"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	"github.com/kelseyhightower/envconfig"
	performance "github.com/openshift-kni/performance-addon-operators/api/v2"
	performancev2 "github.com/openshift-kni/performance-addon-operators/api/v2"
	mcov1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	mcv1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	mcoScheme "github.com/openshift/machine-config-operator/pkg/generated/clientset/versioned/scheme"
	ptpv1 "github.com/openshift/ptp-operator/pkg/apis/ptp/v1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/cnf-tests/discovery/parameters"
	. "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	generalParameters "gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/machineconfigpool"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func envVarErrorString(envVarName string) error {
	return fmt.Errorf("error to detect %s env var", envVarName)
}

// NewConfig reads env var and sort them in to struct
func NewConfig() (*parameters.EnvironmentConfig, error) {
	var environmentConfiguration parameters.EnvironmentConfig
	err := envconfig.Process("", &environmentConfiguration)
	if err != nil {
		return nil, err
	}

	if environmentConfiguration.CnfTestImage == "" {
		return nil, envVarErrorString("CNF_IMAGE_VERSION")
	}

	if environmentConfiguration.DpdkTestImage == "" {
		return nil, envVarErrorString("DPDK_IMAGE_VERSION")
	}

	if environmentConfiguration.TestImageRegistry == "" {
		return nil, envVarErrorString("CONTAINER_REPO")
	}

	return &environmentConfiguration, nil
}

// CreatePerformanceProfile creates performance profile
func CreatePerformanceProfile(performanceProfileName string, mcpPoolName string) error {
	isolatedCPUSet := performancev2.CPUSet("8-15")
	reservedCPUSet := performancev2.CPUSet("0-7")
	hugepageSize := performancev2.HugePageSize("1G")
	performanceProfile := &performancev2.PerformanceProfile{
		ObjectMeta: metav1.ObjectMeta{
			Name: performanceProfileName,
		},
		Spec: performancev2.PerformanceProfileSpec{
			CPU: &performancev2.CPU{
				Isolated: &isolatedCPUSet,
				Reserved: &reservedCPUSet,
			},
			HugePages: &performancev2.HugePages{
				DefaultHugePagesSize: &hugepageSize,
				Pages: []performancev2.HugePage{
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

// WaitForClusterToBeStable validates if MCP is stable
func WaitForClusterToBeStable(machineConfigPoolName string) error {
	mcp := &mcv1.MachineConfigPool{}
	err := Apiclient.Client.Get(context.TODO(), goclient.ObjectKey{Name: machineConfigPoolName}, mcp)
	if err != nil {
		return err
	}

	err = machineconfigpool.WaitForCondition(
		Apiclient,
		&mcv1.MachineConfigPool{ObjectMeta: metav1.ObjectMeta{Name: machineConfigPoolName}},
		mcv1.MachineConfigPoolUpdating,
		corev1.ConditionTrue,
		2*time.Minute)
	if err != nil {
		return err
	}

	// We need to wait a long time here for the node to reboot
	err = machineconfigpool.WaitForCondition(
		Apiclient,
		&mcv1.MachineConfigPool{ObjectMeta: metav1.ObjectMeta{Name: machineConfigPoolName}},
		mcv1.MachineConfigPoolUpdated,
		corev1.ConditionTrue,
		time.Duration(20*mcp.Status.MachineCount)*time.Minute)

	return err
}

// CreatePTPConfig creates PtpConfig based on provided resource
func CreatePTPConfig(profileName string,
	ptpOperatorNamespace string,
	ifaceName string,
	ptp4lOpts,
	phc2sysOpts,
	nodeLabel string,
	priority *int64) error {

	var ptpProfile []ptpv1.PtpProfile
	var ptpRecommend []ptpv1.PtpRecommend
	matchRule := ptpv1.MatchRule{NodeLabel: &nodeLabel}
	ptpProfile = append(
		ptpProfile, ptpv1.PtpProfile{
			Name:        &profileName,
			Interface:   &ifaceName,
			Phc2sysOpts: &phc2sysOpts,
			Ptp4lOpts:   &ptp4lOpts})

	ptpRecommend = append(
		ptpRecommend,
		ptpv1.PtpRecommend{
			Profile:  &profileName,
			Priority: priority,
			Match:    []ptpv1.MatchRule{matchRule}})

	policy := ptpv1.PtpConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      profileName,
			Namespace: ptpOperatorNamespace},
		Spec: ptpv1.PtpConfigSpec{
			Profile:   ptpProfile,
			Recommend: ptpRecommend}}

	_, err := Apiclient.PtpConfigs(ptpOperatorNamespace).Create(context.Background(), &policy, metav1.CreateOptions{})
	return err
}

// DeployMC installs MachineConfig based on provided resource definition
func DeployMC(machineConfig string) error {
	mc, err := DecodeMCYaml(machineConfig)
	if err != nil {
		return err
	}
	err = Apiclient.Client.Create(context.TODO(), mc)
	if err != nil {
		return err
	}
	return nil
}

// DecodeMCYaml decodes a MachineConfig YAML to a MachineConfig struct
func DecodeMCYaml(mcyaml string) (*mcov1.MachineConfig, error) {
	decode := mcoScheme.Codecs.UniversalDeserializer().Decode
	obj, _, err := decode([]byte(mcyaml), nil, nil)
	if err != nil {
		return nil, err
	}
	mc, ok := obj.(*mcov1.MachineConfig)
	if !ok {
		return nil, fmt.Errorf("couldnt create MC object from mcyaml")
	}

	return mc, err
}

// CleanAllSriovPolicy removes all SriovNetworkNodePolicyList except default
func CleanAllSriovPolicy() error {
	sriovNodePolicyList := &sriovv1.SriovNetworkNodePolicyList{}
	err := Apiclient.Client.List(context.TODO(), sriovNodePolicyList)
	if err != nil {
		return err
	}
	if len(sriovNodePolicyList.Items) > 1 {
		for _, sriovNodePolicy := range sriovNodePolicyList.Items {
			if sriovNodePolicy.Name != "default" {
				err := Apiclient.Client.Delete(
					context.TODO(),
					&sriovNodePolicy)
				if err != nil {
					return err
				}
			}
		}
		WaitForSRIOVStable(generalParameters.SriovOperatorNamespace, parameters.SriovWaitingTime)
	}
	return nil
}

// CleanAllPerformanceProfile removes all PerformanceProfile from cluster
func CleanAllPerformanceProfile(cnfNodeLabel string) error {
	performanceProfileList := &performance.PerformanceProfileList{}
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
		err = WaitForClusterToBeStable(cnfNodeLabel)
		if err != nil {
			return err
		}
	}
	return nil
}

// DefineSCTPMC returns SCTP MachineConfig string
func DefineSCTPMC(roleWorker string) string {
	return fmt.Sprintf(`
apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
  labels:
    machineconfiguration.openshift.io/role: %s
  name: load-sctp-module
spec:
  config:
    ignition:
      version: 2.2.0
    storage:
      files:
        - contents:
            source: data:,
            verification: {}
          filesystem: root
          mode: 420
          path: /etc/modprobe.d/sctp-blacklist.conf
        - contents:
            source: data:text/plain;charset=utf-8,sctp
          filesystem: root
          mode: 420
          path: /etc/modules-load.d/sctp-load.conf
`, roleWorker)
}

// DefineXtu32MC returns XT_u32 MachineConfig string
func DefineXtu32MC(roleWorker string) string {
	return fmt.Sprintf(`apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
  labels:
    machineconfiguration.openshift.io/role: %s
  name: load-xt-u32-module
spec:
  config:
    ignition:
      version: 2.2.0
    storage:
      files:
        - contents:
            source: data:text/plain;charset=utf-8,xt_u32
          filesystem: root
          mode: 420
          path: /etc/modules-load.d/xt_u32-load.conf`, roleWorker)
}
