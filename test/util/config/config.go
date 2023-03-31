package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/kelseyhightower/envconfig"
	"gopkg.in/yaml.v2"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"

	testclient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
)

const (
	// PathToConfig path to config file.
	PathToConfig      = "config/config.yaml"
	PathToPodExecLogs = "/tmp/pod_exec_logs.log"
)

// Config type keeps general configuration.
type Config struct {
	General struct {
		ReportDirAbsPath              string `yaml:"report" envconfig:"REPORT_DIR_NAME"`
		CnfNodeLabel                  string `yaml:"cnf_worker_label" envconfig:"ROLE_WORKER_CNF"`
		DumpFailedTestsReportLocation string `envconfig:"REPORTER_ERROR_OUTPUT"`
		PolarionReport                bool   `yaml:"polarion_report" envconfig:"POLARION_REPORT"`
	} `yaml:"general"`
	Network struct {
		TestContainerImage      string `yaml:"test_container_image" envconfig:"NETWORK_TEST_CONTAINER_IMAGE"`
		SriovInterfaces         string `envconfig:"CNF_INTERFACES_LIST"`
		MetalLBAddressPoolIP    string `envconfig:"METALLB_ADDR_LIST"`
		MetalLBSwitchInterfaces string `envconfig:"METALLB_SWITCH_INTERFACES"`
		MetalLBVlanIDs          string `envconfig:"METALLB_VLANS"`
		FrrImage                string `yaml:"frr_image" envconfig:"FRR_IMAGE"`
		SwitchUser              string `envconfig:"SWITCH_USER"`
		SwitchPass              string `envconfig:"SWITCH_PASS"`
		SwitchIP                string `envconfig:"SWITCH_IP"`
		SwitchInterfaces        string `envconfig:"SWITCH_INTERFACES"`
	} `yaml:"network"`
	Ran struct {
		CnfTestImage              string   `yaml:"cnf_test_image" envconfig:"CNF_TEST_IMAGE"`
		StressngTestImage         string   `yaml:"stressng_test_image" envconfig:"STRESSNG_TEST_IMAGE"`
		OslatTestImage            string   `yaml:"oslat_test_image" envconfig:"OSLAT_TEST_IMAGE"`
		ProcessExporterImage      string   `yaml:"process_exporter_image" envconfig:"PROCESS_EXPORTER_IMAGE"`
		ProcessExporterConfigsDir string   `yaml:"process_exporter_resources"`
		BmcHosts                  string   `envconfig:"BMC_HOSTS"`
		BmcUser                   string   `yaml:"bmc_user" envconfig:"BMC_USER"`
		BmcPassword               string   `yaml:"bmc_password" envconfig:"BMC_PASSWORD"`
		PduAddr                   string   `envconfig:"PDU_ADDR"`
		PduSocket                 string   `envconfig:"PDU_SOCKET"`
		RanEventTestDebug         string   `envconfig:"RAN_EVENT_TEST_DEBUG"`
		BmerConsumerImage         string   `yaml:"bmer_consumer_image" envconfig:"BMER_CONSUMER_IMAGE"`
		BmerConfigsDir            string   `yaml:"bmer_consumer_manifests"`
		KubeconfigHub             string   `envconfig:"KUBECONFIG_HUB"`
		OcpUpgradeUpstreamURL     string   `yaml:"ocp_upgrade_upstream_url" envconfig:"OCP_UPGRADE_UPSTREAM_URL"`
		TalmPrecachePolicies      []string `envconfig:"TALM_PRECACHE_POLICIES" yaml:"talm_precache_policies"`
		KubeconfigSpoke2          string   `envconfig:"KUBECONFIG_SPOKE2"`
		ZtpGitRepo                string   `envconfig:"ZTP_GIT_REPO"`
		ZtpGitBranch              string   `envconfig:"ZTP_GIT_BRANCH"`
		ZtpGitDir                 string   `envconfig:"ZTP_GIT_DIR"`
	} `yaml:"ran"`
}

// NewConfig returns instance Config type.
func NewConfig() (*Config, error) {
	var conf Config

	_, filename, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(filepath.Dir(filepath.Join(filepath.Dir(filename), "..")))
	confFile := filepath.Join(baseDir, PathToConfig)
	err := readFile(&conf, confFile)

	if err != nil {
		return nil, err
	}

	conf.General.ReportDirAbsPath = filepath.Join(baseDir, conf.General.ReportDirAbsPath)
	err = deployReportDir(confFile, conf.General.ReportDirAbsPath, "report", "REPORT_DIR_NAME")

	if err != nil {
		return nil, err
	}

	conf.Ran.ProcessExporterConfigsDir = filepath.Join(baseDir, conf.Ran.ProcessExporterConfigsDir)
	err = readEnv(&conf)

	if err != nil {
		return nil, err
	}

	return &conf, nil
}

func readFile(conf *Config, cfgfile string) error {
	openedCfgFile, err := os.Open(cfgfile)
	if err != nil {
		return err
	}

	defer func() {
		_ = openedCfgFile.Close()
	}()

	decoder := yaml.NewDecoder(openedCfgFile)
	err = decoder.Decode(&conf)

	if err != nil {
		return err
	}

	return nil
}

func readEnv(c *Config) error {
	err := envconfig.Process("", c)
	if err != nil {
		return err
	}

	return nil
}

// GetReportPath returns full path to the report file.
func (c *Config) GetReportPath(file string) string {
	reportFileName := strings.TrimSuffix(filepath.Base(file), filepath.Ext(filepath.Base(file)))

	return fmt.Sprintf("%s.xml", filepath.Join(c.General.ReportDirAbsPath, reportFileName))
}

// GetPolarionReportPath returns full path to the polarion report file.
func (c *Config) GetPolarionReportPath() string {
	reportFileName := strings.TrimSuffix(filepath.Base("report"), filepath.Ext(filepath.Base("report")))

	if !c.General.PolarionReport {
		return ""
	}

	return fmt.Sprintf("%s_polarion.xml", filepath.Join(c.General.ReportDirAbsPath, reportFileName))
}

// GetCnfInterfaces returns list of requested interfaces.
func (c *Config) GetCnfInterfaces(requestedNumber int) ([]string, error) {
	if c.Network.SriovInterfaces == "" {
		return nil, fmt.Errorf("environment variable CNF_INTERFACES_LIST is not set")
	}

	requestedInterfaceList := strings.Split(c.Network.SriovInterfaces, ",")

	if len(requestedInterfaceList) < requestedNumber {
		return nil, fmt.Errorf("CNF_INTERFACES_LIST has less interfaces than requested by test suite")
	}

	return requestedInterfaceList, nil
}

// GetSriovInterfaces returns list of requested interfaces.
func (c *Config) GetSriovInterfaces(
	availableSriovInterfaces []*sriovv1.InterfaceExt, requestedNumber int) ([]*sriovv1.InterfaceExt, error) {
	var validSriovIntefaceList []*sriovv1.InterfaceExt

	requestedSriovInterfaceList, err := c.GetCnfInterfaces(requestedNumber)

	if err != nil {
		return nil, err
	}

	for _, availableSriovInterface := range availableSriovInterfaces {
		for _, requestedSriovInterface := range requestedSriovInterfaceList {
			if availableSriovInterface.Name == requestedSriovInterface {
				validSriovIntefaceList = append(validSriovIntefaceList, availableSriovInterface)
			}
		}
	}

	if len(validSriovIntefaceList) < requestedNumber {
		return nil, fmt.Errorf("requested interfaces %v are not present on cluster node", requestedSriovInterfaceList)
	}

	return validSriovIntefaceList, nil
}

// GetDumpFailedTestReportLocation returns destination file for failed tests logs.
func (c *Config) GetDumpFailedTestReportLocation(file string) string {
	if c.General.DumpFailedTestsReportLocation == "true" {
		if _, err := os.Stat(c.General.ReportDirAbsPath); os.IsNotExist(err) {
			err := os.MkdirAll(c.General.ReportDirAbsPath, 0744)
			if err != nil {
				panic(fmt.Errorf("can not create report dir due to %w", err))
			}
		}

		dumpFileName := strings.TrimSuffix(filepath.Base(file), filepath.Ext(filepath.Base(file)))

		return filepath.Join(c.General.ReportDirAbsPath, fmt.Sprintf("failed_%s", dumpFileName))
	}

	return ""
}

// DefineClients sets client and return it's instance.
func DefineClients() (*testclient.ClientSet, error) {
	clients := testclient.New("")
	if clients == nil {
		return nil, fmt.Errorf("client is not set please check KUBECONFIG env variable")
	}

	return clients, nil
}

// GetMetallbVirtIP IPv4 checks the environmental variable and returns the value in []string.
func (c *Config) GetMetallbVirtIP() ([]string, error) {
	envValue := strings.Split(c.Network.MetalLBAddressPoolIP, ",")
	if len(envValue) < 2 {
		return nil, fmt.Errorf("there are not enough environment IP variables")
	}

	for _, v := range envValue {
		if net.ParseIP(v) == nil {
			return nil, fmt.Errorf("the environment IP variable is not a valid IP")
		}
	}

	return envValue, nil
}

// GetSwitchUser checks the environmental variable SwitchUser and returns the value in string.
func (c *Config) GetSwitchUser() (string, error) {
	if c.Network.SwitchUser == "" {
		return "", fmt.Errorf("the username for a switch is empty")
	}

	return c.Network.SwitchUser, nil
}

// GetSwitchIP checks the environmental variable SwitchIP and returns the value in string.
func (c *Config) GetSwitchIP() (string, error) {
	if net.ParseIP(c.Network.SwitchIP) == nil {
		return "", fmt.Errorf("the environment switch IP variable is not a valid IP")
	}

	return c.Network.SwitchIP, nil
}

// GetSwitchPass checks the environmental variable SwitchPass and returns the value in string.
func (c *Config) GetSwitchPass() (string, error) {
	if c.Network.SwitchPass == "" {
		return "", fmt.Errorf("the password for a switch is empty")
	}

	return c.Network.SwitchPass, nil
}

func (c *Config) GetMetalLbVlanIds() ([]uint16, error) {
	envValue := strings.Split(c.Network.MetalLBVlanIDs, ",")

	if len(envValue) != 2 {
		return nil, fmt.Errorf("check METALLB_VLANS env var. It reuires two vlans")
	}

	var vlanIds []uint16

	for _, vlan := range envValue {
		vlanID, err := strconv.Atoi(vlan)
		if err != nil {
			return nil, fmt.Errorf("vlan id %s should be interger", vlan)
		}

		if uint16(vlanID) > 4095 {
			return nil, fmt.Errorf("vlan id %s should be less that 4095", vlan)
		}

		vlanIds = append(vlanIds, uint16(vlanID))
	}

	return vlanIds, nil
}

// GetSwitchInterfaces  checks the environmental variable and returns the value in []string.
func (c *Config) GetSwitchInterfaces() ([]string, error) {
	return c.getSwitchInterfacesForSuite("sriov")
}

// GetMetalLbSwitchInterfaces checks the metalLb switch port environmental variable and returns the value in []string.
func (c *Config) GetMetalLbSwitchInterfaces() ([]string, error) {
	return c.getSwitchInterfacesForSuite("metallb")
}

// GetSwitchInterfacesForSuite checks the switch interface environmental variable for specific suite and returns
// the value in []string.
func (c *Config) getSwitchInterfacesForSuite(suiteName string) ([]string, error) {
	metalLbSuiteName, srIovSuiteName := "metallb", "sriov"

	var (
		envVarName string
		envValue   []string
	)

	switch suiteName {
	case metalLbSuiteName:
		envValue = strings.Split(c.Network.MetalLBSwitchInterfaces, ",")
		envVarName = "METALLB_SWITCH_INTERFACES"
	case srIovSuiteName:
		envValue = strings.Split(c.Network.SwitchInterfaces, ",")
		envVarName = "SWITCH_INTERFACES"
	default:
		return nil, fmt.Errorf("invalid suiteName, supported suites are: %s, %s", metalLbSuiteName, srIovSuiteName)
	}

	if len(envValue) == 0 {
		return nil, fmt.Errorf("the environment variable %s is empty", envVarName)
	}

	return envValue, nil
}

func deployReportDir(confFileName string, dirName string, yamlTag string, envVar string) error {
	_, err := os.Stat(dirName)

	if os.IsNotExist(err) {
		return os.MkdirAll(dirName, 0777)
	}

	if err != nil {
		return fmt.Errorf(
			"error in verifying the %s directory. Check if either %s is present in %s or "+
				"%s env var is set", dirName, yamlTag, envVar, confFileName)
	}

	return err
}
