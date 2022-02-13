package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/kelseyhightower/envconfig"
	"gopkg.in/yaml.v2"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"

	testclient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
)

const (
	// PathToConfig path to config file.
	PathToConfig = "config/config.yaml"
)

// Config type keeps general configuration.
type Config struct {
	General struct {
		ReportDirAbsPath              string `yaml:"report" envconfig:"REPORT_DIR_NAME"`
		CnfNodeLabel                  string `yaml:"cnf_worker_label" envconfig:"ROLE_WORKER_CNF"`
		DumpFailedTestsReportLocation string `envconfig:"REPORTER_ERROR_OUTPUT"`
	} `yaml:"general"`
	Network struct {
		TestContainerImage     string `yaml:"test_container_image" envconfig:"NETWORK_TEST_CONTAINER_IMAGE"`
		SriovInterfaces        string `envconfig:"CNF_INTERFACES_LIST"`
		MetalLBAddressPoolIP   string `envconfig:"METALLB_ADDR_LIST"`
		FrrImage               string `yaml:"frr_image" envconfig:"FRR_IMAGE"`
		MetalLBAddressPoolIPV4 string `envconfig:"METALLB_ADDRV4_LIST"`
		MetalLBAddressPoolIPV6 string `envconfig:"METALLB_ADDRV6_LIST"`
		MetalLBDeployIP        string `envconfig:"DEPLOY_IP"`
	} `yaml:"network"`
	Ran struct {
		CnfTestImage              string `yaml:"cnf_test_image" envconfig:"CNF_TEST_IMAGE"`
		StressngTestImage         string `yaml:"stressng_test_image" envconfig:"STRESSNG_TEST_IMAGE"`
		OslatTestImage            string `yaml:"oslat_test_image" envconfig:"OSLAT_TEST_IMAGE"`
		ProcessExporterImage      string `yaml:"process_exporter_image" envconfig:"PROCESS_EXPORTER_IMAGE"`
		ProcessExporterConfigsDir string `yaml:"process_exporter_resources"`
		BmcHosts                  string `envconfig:"BMC_HOSTS"`
		BmcUser                   string `yaml:"bmc_user" envconfig:"BMC_USER"`
		BmcPassword               string `yaml:"bmc_password" envconfig:"BMC_PASSWORD"`
	} `yaml:"ran"`
}

// NewConfig returns instance Config type.
func NewConfig() (*Config, error) {
	var c Config

	_, filename, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(filepath.Dir(filepath.Join(filepath.Dir(filename), "..")))
	confFile := filepath.Join(baseDir, PathToConfig)
	err := readFile(&c, confFile)

	if err != nil {
		return nil, err
	}

	c.General.ReportDirAbsPath = filepath.Join(baseDir, c.General.ReportDirAbsPath)
	c.Ran.ProcessExporterConfigsDir = filepath.Join(baseDir, c.Ran.ProcessExporterConfigsDir)
	err = readEnv(&c)

	if err != nil {
		return nil, err
	}

	return &c, nil
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

// GetSriovInterfaces returns list of requested interfaces.
func (c *Config) GetSriovInterfaces(
	availableSriovInterfaces []*sriovv1.InterfaceExt, requestedNumber int) ([]*sriovv1.InterfaceExt, error) {
	var validSriovIntefaceList []*sriovv1.InterfaceExt

	if c.Network.SriovInterfaces == "" {
		return nil, fmt.Errorf("environment variable CNF_INTERFACES_LIST is not set")
	}

	requestedSriovInterfaceList := strings.Split(c.Network.SriovInterfaces, ",")

	if len(requestedSriovInterfaceList) < requestedNumber {
		return nil, fmt.Errorf("CNF_INTERFACES_LIST has less interfaces than requested by test suite")
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
func (c *Config) GetMetallbVirtIPv4() ([]string, error) {
	envValue := strings.Split(c.Network.MetalLBAddressPoolIPV4, ",")
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

// GetMetallbVirtIP IPv6 checks the environmental variable and returns the value in []string.
func (c *Config) GetMetallbVirtIPv6() ([]string, error) {
	envValue := strings.Split(c.Network.MetalLBAddressPoolIPV6, ",")
	if len(envValue) < 2 {
		return nil, nil
	}

	for _, v := range envValue {
		if net.ParseIP(v) == nil {
			return nil, fmt.Errorf("the environment IP variable is not a valid IP")
		}
	}

	return envValue, nil
}

// GetEnvIPStack IPv4 checks the environmental variable and returns the value in []string.
func (c *Config) GetEnvIPStack() (string, error) {
	if len(c.Network.MetalLBDeployIP) < 1 {
		return "", nil
	}

	return c.Network.MetalLBDeployIP, nil
}
