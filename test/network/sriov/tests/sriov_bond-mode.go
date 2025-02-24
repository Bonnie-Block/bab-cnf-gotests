package tests

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/netsriovhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/netsriovparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/polarion"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CNF SRIOV: Bond CNI.", func() {
	describe := netsriovhelper.DescribeSRIOVParameters
	describeActiveActive := netsriovhelper.DescribeActiveActiveBondParameters
	var (
		sriovInfos *cluster.EnabledNodes
		err        error
		testFail   string
	)

	execute.BeforeAll(func() {
		netsriovhelper.VerifySriovOperatorInstalledAndPreconfigured(namespace, operatorGroup, sriovSubscription)
		By("Discover SRIOV interfaces")
		sriovInfos, err = cluster.DiscoverSriov(
			Apiclient,
			parameters.SriovOperatorNamespace)
		if err != nil {
			testFail = fmt.Sprintf("Error discover SRIOV node info: %s", err)
			Expect(err).ToNot(HaveOccurred(), testFail)
		}
	})

	AfterEach(func() {
		By("Cleaning up resources after test")
		err = namespaces.CleanPodAndWaitUntilItsEmpty(Apiclient, netsriovparameters.OperatorTestNamespace)
		Expect(err).ToNot(HaveOccurred())
	})

	Context("Bond-mode:", func() {
		BeforeEach(func() {
			if testFail != "" {
				Fail(testFail)
			}
		})

		AfterEach(func() {
			err := nethelper.DeleteNADs(netsriovparameters.OperatorTestNamespace,
				netsriovparameters.BondNadName)
			Expect(err).ToNot(HaveOccurred())
			if len(netsriovhelper.InterfaceConfigs) > 0 {
				recoverSwitchConfiguration()
			}
		})

		DescribeTable(
			netsriovparameters.BondModeActiveBackup, polarion.ID("47074"),
			func(mtu int, protocol string, connectivity string, bond bool) {
				netsriovhelper.TestBondScenario(
					mtu,
					sriovInfos,
					protocol,
					connectivity,
					netsriovparameters.BondModeActiveBackup,
					netsriovparameters.ServerPodIP,
					netsriovparameters.ClientPodIP,
					netsriovparameters.IpamStatic)
			},
			netsriovhelper.BuildTableEntries(
				sriovSmokeTestMode,
				describe,
				true,
				[]int{
					netsriovparameters.MTUCustom,
					netsriovparameters.MTUJumbo,
					netsriovparameters.MTUStandard,
				},
				[]string{
					netsriovparameters.ConnectivityDiffNodeDiffPF,
					netsriovparameters.ConnectivityDiffNodeSamePF,
				},
				[]string{
					netsriovparameters.CommunicationProtocolUnicastICMP,
					netsriovparameters.CommunicationProtocolUnicastTCP,
				},
			),
		)

		DescribeTable("Active-Active Bond modes", polarion.ID("47075"),
			func(mtu int, protocol string, bondMode string, bond bool) {
				netsriovhelper.TestActiveActiveBondScenario(
					mtu,
					sriovInfos,
					protocol,
					bondMode,
					netsriovparameters.ServerPodIP,
					netsriovparameters.ClientPodIP)
			},
			BuildBondTableEntries(
				sriovSmokeTestMode,
				describeActiveActive,
				true,
				[]int{
					netsriovparameters.MTUCustom,
					netsriovparameters.MTUJumbo,
					netsriovparameters.MTUStandard,
				},
				[]string{
					netsriovparameters.BondModeRR,
					netsriovparameters.BondModeXOR,
				},
				[]string{
					netsriovparameters.CommunicationProtocolUnicastICMP,
					netsriovparameters.CommunicationProtocolUnicastTCP,
				},
			))
	})

	Context("Scale: Bond with 16 VFs", func() {
		var (
			clientTestCommand   []string
			clientPod           *corev1.Pod
			totalNumberSlaveVFs = 16
			slaveNetworks       []string
		)
		BeforeEach(func() {
			if testFail != "" {
				Fail(testFail)
			}
			sriovInterfaces, err := sriovInfos.FindSriovDevices(sriovInfos.Nodes[0])
			Expect(err).ToNot(HaveOccurred())
			validSriovInterfaces, err := Config.GetSriovInterfaces(sriovInterfaces, 2)
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error determine SRIOV interfaces: %s", err))

			isScaleSupported := netsriovhelper.IsScaleSupported(netsriovparameters.ScaleVFsNumber,
				validSriovInterfaces)
			if !isScaleSupported {
				Skip(fmt.Sprintf("Requested interfaces %v are not supported scale VFs number - %d",
					validSriovInterfaces, netsriovparameters.ScaleVFsNumber))
			}

			By("Creating Bond interface")
			ScaleNADBond, err := netsriovhelper.DefineBondNad(netsriovparameters.BondNadName,
				netsriovparameters.BondModeActiveBackup,
				netsriovparameters.MTUStandard,
				totalNumberSlaveVFs, netsriovparameters.IpamStatic)
			Expect(err).ToNot(HaveOccurred())

			err = Apiclient.Create(context.Background(), ScaleNADBond)
			Expect(err).ToNot(HaveOccurred())

			// Creating half of the slave VFs from one network and the other half from another network
			for i := 0; i < totalNumberSlaveVFs/2; i++ {
				slaveNetworks = append(slaveNetworks, netsriovparameters.SriovScaleBondName)
			}

			for i := 0; i < totalNumberSlaveVFs/2; i++ {
				slaveNetworks = append(slaveNetworks, netsriovparameters.SriovScaleBondNameDiff)
			}

			By("Creating Server Pod")
			netsriovhelper.RunServerPod(
				netsriovparameters.CommunicationProtocolUnicastICMP,
				netsriovparameters.MTUStandard,
				netsriovparameters.ConnectivityDiffNodeDiffPF,
				sriovInfos,
				Config,
				netsriovparameters.BondNadName,
				slaveNetworks,
				false,
				"",
				netsriovparameters.ServerPodIP,
				netsriovparameters.TestBondInterfaceName,
				netsriovparameters.IpamStatic)

			By("Creating Client Pod")
			clientPodDefinition := netsriovhelper.DefineClientPod(
				netsriovparameters.CommunicationProtocolUnicastICMP,
				sriovInfos.Nodes,
				netsriovparameters.BondNadName,
				slaveNetworks,
				netsriovparameters.ClientPodIP,
				"",
				Config.Network.TestContainerImage,
				parameters.SleepCommand,
				netsriovparameters.IpamStatic,
				netsriovparameters.TestBondInterfaceName)

			clientTestCommand, err = netsriovhelper.DefineTestCommandParameters(
				false,
				netsriovparameters.CommunicationProtocolUnicastICMP,
				netsriovparameters.MTUStandard,
				netsriovparameters.ServerPodIP,
				netsriovparameters.TestPort,
				netsriovparameters.TestBondInterfaceName)
			Expect(err).ToNot(HaveOccurred())

			clientPod, err = Apiclient.Pods(netsriovparameters.OperatorTestNamespace).Create(
				context.Background(),
				clientPodDefinition,
				metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
			netsriovhelper.WaitUntilPodInStatus(
				clientPod,
				"Client",
				parameters.SleepCommand,
				corev1.PodRunning,
				netsriovparameters.PodWaitingTime)

			isBondInterfaceUp, err := netsriovhelper.BondInterfaceIsUp(clientPod,
				netsriovparameters.TestBondInterfaceName)
			Expect(err).ToNot(HaveOccurred())
			Expect(isBondInterfaceUp).To(BeTrue(), "Bond interface is not Up")

			isBondInterfaceHasInMode, err := netsriovhelper.BondInterfaceHasMode(clientPod,
				netsriovparameters.TestBondInterfaceName,
				netsriovparameters.BondModeActiveBackup)
			Expect(err).ToNot(HaveOccurred())
			Expect(isBondInterfaceHasInMode).To(BeTrue(), "Bond interface has incorrect bond type")

			isBondInterfaceHasSlaves, err := netsriovhelper.BondInterfaceHasSlaves(clientPod,
				netsriovparameters.TestBondInterfaceName,
				"16")
			Expect(err).ToNot(HaveOccurred())
			Expect(isBondInterfaceHasSlaves).To(BeTrue(), "Bond interface has wrong number of slaves")
		})

		AfterEach(func() {
			err := nethelper.DeleteNADs(netsriovparameters.OperatorTestNamespace, netsriovparameters.BondNadName)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should work with ICMP traffic", polarion.ID("47067"), func() {
			_, err = pod.ExecCommand(Apiclient, *clientPod, clientTestCommand)
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to run ICMP traffic - %s", err))
		})
	})
})

func recoverSwitchConfiguration() {
	switchCredentials, err := nethelper.NewSwitchCredentials()
	Expect(err).ToNot(HaveOccurred())
	switchInterfaces, err := Config.GetSwitchInterfaces()
	Expect(err).ToNot(HaveOccurred())
	switchLagNames, err := Config.GetSwitchLagNames()
	Expect(err).ToNot(HaveOccurred())

	err = netsriovhelper.RestoreSwitchInterfacesConfiguration(switchCredentials, switchInterfaces)
	Expect(err).ToNot(HaveOccurred())

	err = netsriovhelper.DeleteNonLACPLAGsOnJunos(switchCredentials,
		[]string{switchLagNames[0], switchLagNames[1]})
	Expect(err).ToNot(HaveOccurred())
}

func BuildBondTableEntries(
	sriovSmokeTestMode bool,
	describe interface{},
	bond bool,
	mtuParameters []int,
	optionParameters []string,
	protocolParameters []string) []TableEntry {
	var tableEntries []TableEntry

	if sriovSmokeTestMode {
		var (
			protocolIndex            int
			mtuIndex                 int
			optionIndex              int
			lenghtOfParametersArrays = []int{
				len(mtuParameters),
				len(optionParameters),
				len(protocolParameters),
			}
			max = lenghtOfParametersArrays[0]
		)

		for _, listLenght := range lenghtOfParametersArrays {
			if listLenght > max {
				max = listLenght
			}
		}

		for i := 0; i < max; i++ {
			if protocolIndex >= len(protocolParameters) {
				protocolIndex = 0
			}

			if mtuIndex >= len(mtuParameters) {
				mtuIndex = 0
			}

			if optionIndex >= len(optionParameters) {
				optionIndex = 0
			}

			tableEntries = append(
				tableEntries,
				Entry(
					describe,
					mtuParameters[mtuIndex],
					protocolParameters[protocolIndex],
					optionParameters[optionIndex],
					bond,
					polarion.SetProperty("MTU", fmt.Sprintf("%d", mtuParameters[optionIndex])),
					polarion.SetProperty("BondMode", optionParameters[optionIndex]),
					polarion.SetProperty("Protocol", protocolParameters[protocolIndex]),
				),
			)
			mtuIndex++
			optionIndex++
			protocolIndex++
		}
	} else {
		for _, protocol := range protocolParameters {
			for _, mtu := range mtuParameters {
				for _, option := range optionParameters {
					tableEntries = append(
						tableEntries,
						Entry(describe, mtu, protocol, option, bond,
							polarion.SetProperty("MTU", fmt.Sprintf("%d", mtu)),
							polarion.SetProperty("BondMode", option),
							polarion.SetProperty("Protocol", protocol)))
				}
			}
		}
	}

	return tableEntries
}
