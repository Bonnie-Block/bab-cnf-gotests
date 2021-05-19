package helper

import (
	"context"
	"fmt"
	"github.com/kelseyhightower/envconfig"
	performancev2 "github.com/openshift-kni/performance-addon-operators/api/v2"
	mcov1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	mcv1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	mcoScheme "github.com/openshift/machine-config-operator/pkg/generated/clientset/versioned/scheme"
	ptpv1 "github.com/openshift/ptp-operator/pkg/apis/ptp/v1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/cnf-tests/discovery/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/machineconfigpool"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"os/exec"
	"path"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"time"
)

func envVarErrorString(envVarName string) error {
	return fmt.Errorf("error to detect %s env var", envVarName)
}
func NewConfig() (*parameters.EnvironmentConfig, error) {
	var myEnv parameters.EnvironmentConfig
	err := envconfig.Process("", &myEnv)
	if err != nil {
		return nil, err
	}

	if myEnv.CnfTestImage == "" {
		return nil,envVarErrorString("CNF_IMAGE_VERSION")
	}

	if myEnv.DpdkTestImage == "" {
		return nil,envVarErrorString("DPDK_IMAGE_VERSION")
	}

	if myEnv.TestImageRegistry == "" {
		return nil,envVarErrorString("CONTAINER_REPO")
	}

	return &myEnv, nil
}

func validateDockerDaemonRunning() error {
	isDaemonRunning := exec.Command("systemctl", "is-active", "--quiet", "docker")
	err := isDaemonRunning.Run()
	if err != nil {
		return fmt.Errorf("docker daemon is not active on host")
	}
	return nil
}

func selectContainerEngine() (*exec.Cmd, error) {
	for _, containerEngine := range []string{"docker", "podman"} {
		containerEngineCMD := exec.Command(containerEngine)
		directoryName, _ := path.Split(containerEngineCMD.Path)
		if directoryName != "" {
			return containerEngineCMD, nil
		}
	}
	return nil, fmt.Errorf("no container Engine present on host machine")
}

func CreatePerformanceProfile(client *client.ClientSet, performanceProfileName string, mcpPoolName string) error {
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
						Count: 5,
						Size:  hugepageSize,
						Node:  pointer.Int32Ptr(0),
					},
				},
			},
			NodeSelector: map[string]string{
				fmt.Sprintf("%s", mcpPoolName): "",
			},
		},
	}
	return client.Client.Create(context.TODO(), performanceProfile)
}

func PullImage(ImageRegistry string, ImageName string) error {
	containerEngine, err := selectContainerEngine()
	if err != nil {
		return err
	}
	if strings.Contains(containerEngine.Path, "docker") {
		err := validateDockerDaemonRunning()
		if err != nil {
			return err
		}
	}
	status := exec.Command(
		containerEngine.Path,
		"pull",
		fmt.Sprintf("%s/%s", ImageRegistry, ImageName))
	_, err = status.Output()
	if err != nil {
		return err
	}

	status = exec.Command(
		containerEngine.Path,
		"images", "--quiet",
		fmt.Sprintf("%s/%s", ImageRegistry, ImageName))

	commandOutput, err := status.Output()
	if err != nil {
		return err
	}
	if string(commandOutput) == "" {
		return fmt.Errorf("error to pull the image")
	}
	return nil
}

func WaitForClusterToBeStable(client *client.ClientSet, machineConfigPoolName string) error {
	mcp := &mcv1.MachineConfigPool{}
	err := client.Client.Get(context.TODO(), goclient.ObjectKey{Name: machineConfigPoolName}, mcp)
	if err != nil {
		return err
	}

	err = machineconfigpool.WaitForCondition(
		client,
		&mcv1.MachineConfigPool{ObjectMeta: metav1.ObjectMeta{Name: machineConfigPoolName}},
		mcv1.MachineConfigPoolUpdating,
		corev1.ConditionTrue,
		2*time.Minute)
	if err != nil {
		return err
	}

	// We need to wait a long time here for the node to reboot
	err = machineconfigpool.WaitForCondition(
		client,
		&mcv1.MachineConfigPool{ObjectMeta: metav1.ObjectMeta{Name: machineConfigPoolName}},
		mcv1.MachineConfigPoolUpdated,
		corev1.ConditionTrue,
		time.Duration(20*mcp.Status.MachineCount)*time.Minute)

	return err
}

func CreatePTPConfig(apiclient *client.ClientSet,
	profileName string,
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
		Name: &profileName,
		Interface: &ifaceName,
		Phc2sysOpts: &phc2sysOpts,
		Ptp4lOpts: &ptp4lOpts})

	ptpRecommend = append(
		ptpRecommend,
		ptpv1.PtpRecommend{
		Profile: &profileName,
		Priority: priority,
		Match: []ptpv1.MatchRule{matchRule}})

	policy := ptpv1.PtpConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: profileName,
			Namespace: ptpOperatorNamespace},
		Spec: ptpv1.PtpConfigSpec{
			Profile: ptpProfile,
			Recommend: ptpRecommend}}

	_, err := apiclient.PtpConfigs(ptpOperatorNamespace).Create(context.Background(), &policy, metav1.CreateOptions{})
	return err
}

func DeploySCTPMc(apiclient *client.ClientSet, roleWorker string) error {
	mcContent := fmt.Sprintf(`
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
	_, err := createMCWithSCTPContent(apiclient, mcContent)
	return err
}

func createMCWithSCTPContent(apiclient *client.ClientSet, mcContent string) (*mcov1.MachineConfig, error) {
	mc, err := DecodeMCYaml(mcContent)
	if err != nil {
		return nil, err
	}

	err = apiclient.Client.Create(context.TODO(), mc)
	return mc, err
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