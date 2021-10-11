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
	DiscoverySriovPolicy                       = "discovery-policy"
	DiscoverySriovPolicyIntel                  = "discovery-policy-intel"
	DiscoveryPerformanceProfile                = "discovery-mode-profile"
	DiscoveryPtpSlaveNodeLabel                 = "ptp/discovery-slave"
	DiscoveryPtpGrandmasterNodeLabel           = "ptp/discovery-grandmaster"
	DiscoveryPtpGrandmasterProfile             = "discovery-master-profile"
	DiscoveryPtpWorkerProfile                  = "discovery-worker-profile"
	JUnitCNFTestsReportName                    = "cnftests-junit.xml"
	SriovWaitingTime                           = 35 * time.Minute
	DiscoveryAllFeaturesScenario               = "discoveryAllFeatures"
	DiscoveryExceptSriovScenario               = "discoveryExceptSriov"
	DiscoveryExceptSriovPtpScenario            = "discoveryExceptSriovPtp"
	DiscoveryExceptSriovPtpPerformanceScenario = "discoveryExceptSriovPtpPerformance"
)

const (
	// DiscoveryAllFeaturesPassedTest expected number of passed tests with
	// SriovNetworkNodePolicy, sctp machine config, xt_u32 machine config, PerformanceProfile, PtpConfig
	// resources configured
	DiscoveryAllFeaturesPassedTest = 74
	// DiscoveryAllFeaturesSkippedTest expected number of skipped tests with
	// SriovNetworkNodePolicy, sctp machine config, xt_u32 machine config, PerformanceProfile, PtpConfig
	// resources configured
	DiscoveryAllFeaturesSkippedTest = 64

	// DiscoveryExceptSriovPassedTest expected number of passed tests with:
	// sctp machine config, xt_u32 machine config, PerformanceProfile, PtpConfig resources configured
	DiscoveryExceptSriovPassedTest = 54
	// DiscoveryExceptSriovSkippedTest expected number of skipped tests with
	// sctp machine config, xt_u32 machine config, PerformanceProfile, PtpConfig resources configured
	DiscoveryExceptSriovSkippedTest = 84

	// DiscoveryExceptSriovPtpPassedTest expected number of passed tests with
	// sctp machine config, xt_u32 machine config, PerformanceProfile resources configured
	DiscoveryExceptSriovPtpPassedTest = 48
	// DiscoveryExceptSriovPtpSkippedTest expected number of skipped tests with
	// sctp machine config, xt_u32 machine config, PerformanceProfile resources configured
	DiscoveryExceptSriovPtpSkippedTest = 84

	// DiscoveryExceptSriovPtpPerformancePassedTest expected number of passed tests with
	// sctp machine config, xt_u32 machine config  resources configured
	DiscoveryExceptSriovPtpPerformancePassedTest = 18
	// DiscoveryExceptSriovPtpPerformancePassedTestSkippedTest expected number of skipped tests with
	// sctp machine config, xt_u32 machine config resources configured
	DiscoveryExceptSriovPtpPerformancetSkippedTest = 120

	// SNODiscoveryAllFeaturesPassedTest expected number of passed tests with
	// SriovNetworkNodePolicy, sctp machine config, xt_u32 machine config, PerformanceProfile
	// resources configured
	// for SNO
	SNODiscoveryAllFeaturesPassedTest = 63
	// SNODiscoveryAllFeaturesSkippedTest expected number of skipped tests with
	// SriovNetworkNodePolicy, sctp machine config, xt_u32 machine config, PerformanceProfile
	// resources configured
	// for SNO
	SNODiscoveryAllFeaturesSkippedTest = 71

	// SNODiscoveryExceptSriovPassedTest expected number of passed tests with:
	// sctp machine config, xt_u32 machine config, PerformanceProfile resources configured
	// for SNO
	SNODiscoveryExceptSriovPassedTest = 43
	// SNODiscoveryExceptSriovSkippedTest expected number of skipped tests with
	// sctp machine config, xt_u32 machine config, PerformanceProfile resources configured
	// for SNO
	SNODiscoveryExceptSriovSkippedTest = 91

	// SNODiscoveryExceptSriovPerformancePassedTest expected number of passed tests with
	// sctp machine config, xt_u32 machine config  resources configured
	// for SNO
	SNODiscoveryExceptSriovPerformancePassedTest = 17
	// SNODiscoveryExceptSriovPerformancePassedTestSkippedTest expected number of skipped tests with
	// sctp machine config, xt_u32 machine config resources configured
	// for SNO
	SNODiscoveryExceptSriovPerformanceSkippedTest = 117
)
