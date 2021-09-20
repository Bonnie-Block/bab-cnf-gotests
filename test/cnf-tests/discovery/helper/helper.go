package helper

import (
	"context"
	"fmt"
	"time"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	"github.com/kelseyhightower/envconfig"
	mcov1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	mcoScheme "github.com/openshift/machine-config-operator/pkg/generated/clientset/versioned/scheme"
	ptpv1 "github.com/openshift/ptp-operator/pkg/apis/ptp/v1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/cnf-tests/discovery/parameters"
	. "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	generalParameters "gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
func CleanAllSriovPolicy(snoTimeoutMultiplier time.Duration) error {
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
		WaitForSRIOVStable(generalParameters.SriovOperatorNamespace, parameters.SriovWaitingTime, snoTimeoutMultiplier)
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

func DefineDiscoverySriovPolicyList(sriovInterface *sriovv1.InterfaceExt) []*sriovv1.SriovNetworkNodePolicy {
	sriovVFNumber := 5
	if sriovInterface.Vendor == "8086" {
		sriovVFNumber = 10
	}
	discoverySriovPolicyList := []*sriovv1.SriovNetworkNodePolicy{DefineSriovPolicy(
		parameters.DiscoverySriovPolicy,
		generalParameters.SriovOperatorNamespace,
		sriovInterface,
		sriovVFNumber,
		"#0-4",
		9000,
		"sriovnic",
		"netdevice")}
	// Mlx device
	if sriovInterface.Vendor == "15b3" {
		discoverySriovPolicyList[0].Spec.IsRdma = true
	}

	// Intel device
	if sriovInterface.Vendor == "8086" {
		discoverySriovPolicyList[0].Spec.DeviceType = "vfio-pci"
		// We need to add additional sriov policy for intel device in order to run sctp-sriov tests cases
		discoverySriovPolicyList = append(discoverySriovPolicyList,
			DefineSriovPolicy(
				parameters.DiscoverySriovPolicyIntel,
				generalParameters.SriovOperatorNamespace,
				sriovInterface,
				sriovVFNumber,
				"#5-9",
				9000,
				"sriovnicintel",
				"netdevice"))
	}
	return discoverySriovPolicyList
}
