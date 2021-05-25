package parameters

import "time"

type (
	EnvironmentConfig struct {
		DpdkTestImage     string `envconfig:"DPDK_IMAGE_VERSION"`
		CnfTestImage      string `envconfig:"CNF_IMAGE_VERSION"`
		TestImageRegistry string `envconfig:"CONTAINER_REPO"`
	}

	//Report is created from XML output by a Ant JUnit task.
	Report struct {
		//Errors is the number of runtime exceptions
		//which occurred when running the test cases.
		Errors int `xml:"errors,attr"`
		//Failures is the number of test cases which produced
		//an invalid result.
		Failures int     `xml:"failures,attr"`
		Name     string  `xml:"name,attr"`
		Tests    int     `xml:"tests,attr"`
		Time     float64 `xml:"time,attr"`
		//Results is all the failed testcases.
		Results TestCases `xml:"testcase"`
	}

	//TestCase represents a failed testcase
	TestCase struct {
		ClassName string   `xml:"classname,attr"`
		Name      string   `xml:"name,attr"`
		Time      float64  `xml:"time,attr"`
		Fail      *Failure `xml:"failure"`
		Skipped   *string  `xml:"skipped"`
	}

	//TestCases represents all the testcases which a class failed.
	//It implements sort.Sort
	TestCases []*TestCase

	//Failure gives details as to why a test case failed.
	Failure struct {
		Message string `xml:"message,attr"`
		Type    string `xml:"type,attr"`
		Value   string `xml:",innerxml"`
	}
)

const (
	DiscoveryPerformanceProfile      = "discovery-mode-profile"
	TestNamespace                    = "discovery-mode-validation"
	DiscoveryPtpSlaveNodeLabel       = "ptp/discovery-slave"
	DiscoveryPtpGrandmasterNodeLabel = "ptp/discovery-grandmaster"
	DiscoveryPtpGrandmasterProfile   = "discovery-master-profile"
	DiscoveryPtpWorkerProfile        = "discovery-worker-profile"
	JUnitCNFTestsReportName          = "cnftests-junit.xml"
	HostnameLabel                    = "kubernetes.io/hostname"
	SriovWaitingTime                 = 35 * time.Minute
	NamespaceDeleteTimeout           = 1800 * time.Second
)

const (
	// DiscoveryAllFeaturesPassedTest expected number of passed tests with
	// SriovNetworkNodePolicy, sctp machine config, xt_u32 machine config, PerformanceProfile, PtpConfig
	// resources configured
	DiscoveryAllFeaturesPassedTest = 67
	// DiscoveryAllFeaturesSkippedTest expected number of skipped tests with
	// SriovNetworkNodePolicy, sctp machine config, xt_u32 machine config, PerformanceProfile, PtpConfig
	// resources configured
	DiscoveryAllFeaturesSkippedTest = 22
	// DiscoveryExceptSriovPassedTest expected number of passed tests with:
	// sctp machine config, xt_u32 machine config, PerformanceProfile, PtpConfig resources configured

	DiscoveryExceptSriovPassedTest = 49
	// DiscoveryExceptSriovSkippedTest expected number of skipped tests with
	// sctp machine config, xt_u32 machine config, PerformanceProfile, PtpConfig resources configured
	DiscoveryExceptSriovSkippedTest = 40
	// DiscoveryExceptSriovPtpPassedTest expected number of passed tests with
	// sctp machine config, xt_u32 machine config, PerformanceProfile resources configured

	DiscoveryExceptSriovPtpPassedTest = 43
	// DiscoveryExceptSriovPtpSkippedTest expected number of skipped tests with
	// sctp machine config, xt_u32 machine config, PerformanceProfile resources configured
	DiscoveryExceptSriovPtpSkippedTest = 46
	// DiscoveryExceptSriovPtpPerformancePassedTest expected number of passed tests with
	// sctp machine config, xt_u32 machine config  resources configured

	DiscoveryExceptSriovPtpPerformancePassedTest = 18
	// DiscoveryExceptSriovPtpPerformancePassedTestSkippedTest expected number of skipped tests with
	// sctp machine config, xt_u32 machine config resources configured
	DiscoveryExceptSriovPtpPerformancePassedTestSkippedTest = 71
)
