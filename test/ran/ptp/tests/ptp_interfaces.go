package tests

import (
	"context"
	"fmt"
	"log"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptphelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptpparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/polarion"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/schemes/ptp/ptpv1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("PTP Events and Metrics - interface down", Ordered, ContinueOnFailure, func() {
	var (
		errBeforeAll         error
		originPtpConfigSpecs = map[string]ptpv1.PtpConfigSpec{}
		configCountsIfce     ranptpparameters.PtpConfigTypeCounter
	)

	BeforeAll(func() {
		originPtpConfigSpecs, configCountsIfce, errBeforeAll = ptpPretestValidations()
	})

	BeforeEach(func() {
		// Any failure in execute.BeforeAll only fails the first test case.
		if errBeforeAll != nil {
			Skip(fmt.Sprintf("Error encountered before the first test started: %v", errBeforeAll.Error()))
		}

		// Ensure ptp clocks are locked before starting any test
		err := ranptphelper.CheckPtpLockState(5*time.Second, 0)
		if err != nil {
			Skip("PTP clocks are not in locked states")
		}
	})

	AfterEach(func() {
		if CurrentSpecReport().State.String() == "skipped" {
			return
		}

		if CurrentSpecReport().Failed() {
			// Best effort print PTP container logs and metrics
			printPTPInfo()
		} else {
			// Best effort print PTP consumer logs
			printConsumerLog()
		}

		log.Println("Restore ptpconfigs to original specs")
		restorePtpConfigs(originPtpConfigSpecs)
		restorePtpInterfaces()

		// Always restore ptpconfigs to original values after each test
		log.Println("Check ptp clocks are in sync")
		err := ranptphelper.CheckPtpLockState(5*time.Minute, 10*time.Second)
		Expect(err).ToNot(HaveOccurred())
	})

	// 49743
	It("should generate events when slave interface goes down and up", polarion.ID("49742"), func() {
		nodeToPtpDaemonPod, err := ranptphelper.NodesToPtpDaemonPods()
		Expect(err).NotTo(HaveOccurred())

		tested := false
		for nodeName, ptpDaemonPod := range nodeToPtpDaemonPod {
			ptpNode, err := ranhelper.GetNodeByName(nodeName)
			Expect(err).NotTo(HaveOccurred())

			interfaces, err := ranptphelper.GetInterfaces(ptpv1.Slave, *ptpNode)
			ifaceGroups := ranptphelper.GetInterfaceGroups(interfaces)
			Expect(err).NotTo(HaveOccurred())

			for _, ifaces := range ifaceGroups {
				if ranptphelper.ContainsOcpInterface(ifaces) {
					log.Println("Avoid bringing down primary interface:", ranptpparameters.OcpInterface)

					continue
				}

				time.Sleep(3 * time.Second)
				log.Printf("Verify ptp events and metrics via ptp pod")
				verifyEventsAndMetricsSlaveInterfaceDownUp(ptpNode, &ptpDaemonPod, &ptpDaemonPod,
					ranptpparameters.CloudEventContainer, ifaces, false)
				tested = true
			}
			// Tests only 1 node
			if tested {
				break
			}
		}

		if !tested {
			Skip("Test skipped to avoid bringing down port used by br-ex interface")
		}
	})

	// 49734
	It("should have no effect when Boundary Clock master interface goes down and up", polarion.ID("49734"), func() {
		if configCountsIfce.BC == 0 {
			Skip("Test requires Boundary Clock configuration")
		}

		nodeToPtpDaemonPod, err := ranptphelper.NodesToPtpDaemonPods()
		Expect(err).NotTo(HaveOccurred())

		tested := false
		for nodeName, ptpDaemonPod := range nodeToPtpDaemonPod {
			ptpNode, err := ranhelper.GetNodeByName(nodeName)
			Expect(err).NotTo(HaveOccurred())

			interfaces, err := ranptphelper.GetInterfaces(ptpv1.Master, *ptpNode)
			if len(interfaces) < 1 {
				continue
			}
			Expect(err).NotTo(HaveOccurred())

			ifaceGroups := ranptphelper.GetInterfaceGroups(interfaces)
			Expect(err).NotTo(HaveOccurred())

			tested = true
			for _, ifaces := range ifaceGroups {
				iface := ifaces[0]
				startTime := time.Now()
				By(fmt.Sprintf("Bring down ptp Boundary Clock master interfaces %v on node %s\n", ifaces,
					ptpNode.Name))
				for _, i := range ifaces {
					err = ranptphelper.SetInterfaceStatus(&ptpDaemonPod, parameters.PtpContainerName, i,
						ranptpparameters.Off, 2)
					Expect(err).NotTo(HaveOccurred())
				}

				By("Validate PTP metric stays in locked after bringing down BC master interface")
				err = ranptphelper.WaitForPtpClockStateMetric(ptpDaemonPod, ranptpparameters.LockedState,
					iface, 45*time.Second, 30*time.Second)
				Expect(err).NotTo(HaveOccurred(), "PTP metric did not stay in locked state after "+
					"bringing down BC master interface")

				By(fmt.Sprintf("Validate NO [HOLDOVER] state transition event is generated after master "+
					"interface  %s goes down on node %s", iface, ptpNode.Name))
				err = ranptphelper.WaitForEvent(&ptpDaemonPod, ranptpparameters.CloudEventContainer,
					"event.sync.ptp-status.ptp-state-change",
					ranptpparameters.EventHoldOver, iface, "/master", startTime, 1*time.Minute)
				Expect(err).To(HaveOccurred(), "event received for clock state change to "+
					"[HOLDOVER] after bringing down BC master interface")

				By(fmt.Sprintf("Bring up ptp Boundary Clock master interfaces %v on node %s\n", ifaces,
					ptpNode.Name))
				for _, i := range ifaces {
					_ = ranptphelper.SetInterfaceStatus(&ptpDaemonPod, parameters.PtpContainerName, i,
						ranptpparameters.On, 1)
				}

				By(fmt.Sprintf("Validate ptp clock state metrics stays in [LOCKED] after master interface"+
					"%s goes up on node %s",
					ifaces, ptpNode.Name))
				err = ranptphelper.WaitForPtpClockStateMetric(ptpDaemonPod, ranptpparameters.LockedState,
					"", 3*time.Minute, 10*time.Second)
				Expect(err).NotTo(HaveOccurred())
			}
			// Test only on one ptp node
			break
		}
		Expect(tested).To(BeTrue(), "No interfaces found for testing while BC is configured")
	})

	// 73093
	It("should change high availability active profile when other nic interface is down", polarion.ID("73093"), func() {
		if configCountsIfce.HA == 0 {
			Skip("Test requires High Availability configuration")
		}

		nodeToPtpDaemonPod, err := ranptphelper.NodesToPtpDaemonPods()
		Expect(err).NotTo(HaveOccurred())

		for nodeName, ptpDaemonPod := range nodeToPtpDaemonPod {
			By("finding the active HA profile")
			// find the right active profile
			statusProfilesMap, err := ranptphelper.BuildStatusProfileNamesMap(ptpDaemonPod)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(statusProfilesMap[ranptpparameters.Active])).Should(Equal(1))

			activeProfileOriginal, err := ranptphelper.GetHaProfile(ranptpparameters.Active)
			Expect(err).NotTo(HaveOccurred())

			inactiveProfileOriginal, err := ranptphelper.GetHaProfile(ranptpparameters.Inactive)
			Expect(err).NotTo(HaveOccurred())

			By("getting slave interface of the active HA profile")
			// find the interface
			ptpNode, err := ranhelper.GetNodeByName(nodeName)
			Expect(err).NotTo(HaveOccurred())
			profileSlaveIfaceMap, err := ranptphelper.BuildProfileSlaveInterfaceMap(*ptpNode)
			Expect(err).NotTo(HaveOccurred())

			activeIface := profileSlaveIfaceMap[activeProfileOriginal[0]]
			Expect(err).NotTo(HaveOccurred())

			inactiveOriginalIface := profileSlaveIfaceMap[inactiveProfileOriginal[0]]
			Expect(err).NotTo(HaveOccurred())

			if ranptphelper.ContainsOcpInterface([]string{activeIface}) {
				log.Println("Avoid bringing down primary interface:", ranptpparameters.OcpInterface)
				Skip("Test skipped to avoid bringing down port used by br-ex interface")
			}

			startTime := time.Now()
			time.Sleep(1 * time.Second)

			By("bringing down active HA slave interface down")
			// bring that interface down
			err = ranptphelper.SetInterfaceStatus(&ptpDaemonPod, parameters.PtpContainerName, activeIface,
				ranptpparameters.Off, 2)
			Expect(err).NotTo(HaveOccurred())

			err = ranptphelper.GetPTPMetrics(ptpDaemonPod)
			Expect(err).NotTo(HaveOccurred())

			By("Validating the activate HA profile changed")
			activeProfileNew, err := ranptphelper.GetHaProfile(ranptpparameters.Active)
			Expect(err).NotTo(HaveOccurred())
			Expect(activeProfileOriginal).NotTo(Equal(activeProfileNew[0]))

			By("Validating the clock state of the original active interface is FREERUN")
			err = ranptphelper.WaitForPtpClockStateMetric(ptpDaemonPod, ranptpparameters.FreeRunState,
				activeIface, 5*time.Minute, 10*time.Second)
			Expect(err).NotTo(HaveOccurred())

			By("Validate no [HOLDOVER] event for original inactive interface received via pod: " + ptpDaemonPod.Name)
			err = ranptphelper.WaitForEvent(&ptpDaemonPod, ranptpparameters.CloudEventContainer,
				"event.sync.ptp-status.ptp-state-change",
				ranptpparameters.EventHoldOver, inactiveOriginalIface, "", startTime, 10*time.Second)
			Expect(err).To(HaveOccurred(),
				fmt.Sprintf("HOLDOVER event is received for original inactive interface %s after HA BC profile"+
					" changed", inactiveOriginalIface))

			By("Validating the clock state of the original inactive interface is LOCKED")
			err = ranptphelper.WaitForPtpClockStateMetric(ptpDaemonPod, ranptpparameters.LockedState,
				inactiveOriginalIface, 5*time.Minute, 10*time.Second)
			Expect(err).NotTo(HaveOccurred())

			By("Validating the clock state of the CLOCK_REALTIME is LOCKED")
			err = ranptphelper.WaitForPtpClockStateMetric(ptpDaemonPod, ranptpparameters.LockedState,
				"CLOCK_REALTIME", 5*time.Minute, 10*time.Second)
			Expect(err).NotTo(HaveOccurred())
		}
	})

	// 73094
	It("should move to FREERUN state when active and inactive interfaces are down", polarion.ID("73094"), func() {
		if configCountsIfce.HA == 0 {
			Skip("Test requires High Availability configuration")
		}

		nodeToPtpDaemonPod, err := ranptphelper.NodesToPtpDaemonPods()
		Expect(err).NotTo(HaveOccurred())

		for nodeName, ptpDaemonPod := range nodeToPtpDaemonPod {
			activeProfiles, err := ranptphelper.GetHaProfile(ranptpparameters.Active)
			Expect(err).NotTo(HaveOccurred())
			inactiveProfiles, err := ranptphelper.GetHaProfile(ranptpparameters.Inactive)
			Expect(err).NotTo(HaveOccurred())

			ptpNode, err := ranhelper.GetNodeByName(nodeName)
			Expect(err).NotTo(HaveOccurred())

			By("getting the active and inactive ha interfaces")
			profileSlaveIfaceMap, err := ranptphelper.BuildProfileSlaveInterfaceMap(*ptpNode)
			Expect(err).NotTo(HaveOccurred())

			activeIface := profileSlaveIfaceMap[activeProfiles[0]]

			var inactiveIfaces []string
			for _, inactiveProfile := range inactiveProfiles {
				inactiveIfaces = append(inactiveIfaces, profileSlaveIfaceMap[inactiveProfile])
			}
			Expect(err).NotTo(HaveOccurred())

			By("checking the interfaces are not ocp interfaces")
			if ranptphelper.ContainsOcpInterface([]string{activeIface}) {
				log.Println("Avoid bringing down primary interface:", ranptpparameters.OcpInterface)
				Skip("Test skipped to avoid bringing down port used by br-ex interface")
			}

			if ranptphelper.ContainsOcpInterface(inactiveIfaces) {
				log.Println("Avoid bringing down primary interface:", ranptpparameters.OcpInterface)
				Skip("Test skipped to avoid bringing down port used by br-ex interface")
			}

			// bring both interface down.
			By("bringing active and inactive interfaces down")
			err = ranptphelper.SetInterfaceStatus(&ptpDaemonPod, parameters.PtpContainerName, activeIface,
				ranptpparameters.Off, 2)
			Expect(err).NotTo(HaveOccurred())
			for _, inactiveIface := range inactiveIfaces {
				err = ranptphelper.SetInterfaceStatus(&ptpDaemonPod, parameters.PtpContainerName, inactiveIface,
					ranptpparameters.Off, 2)
				Expect(err).NotTo(HaveOccurred())
			}

			By("validating ptp4l clock states are FREERUN")
			slaveIfaces, err := ranptphelper.GetInterfaces(ptpv1.Slave, *ptpNode)
			Expect(err).NotTo(HaveOccurred())

			for _, slaveIface := range slaveIfaces {
				err = ranptphelper.WaitForPtpClockStateMetric(ptpDaemonPod, ranptpparameters.FreeRunState,
					slaveIface, time.Minute, 10*time.Second)
				Expect(err).NotTo(HaveOccurred())
			}

			By("validating CLOCK_REALTIME changes to FREERUN")
			err = ranptphelper.WaitForPtpClockStateMetric(ptpDaemonPod, ranptpparameters.FreeRunState,
				"CLOCK_REALTIME", time.Minute, 10*time.Second)
			Expect(err).NotTo(HaveOccurred())
		}
	})

	Context("remove active profile bc configuration", func() {
		var (
			ptpConfig           ptpv1.PtpConfig
			nodeToPtpDaemonPod  map[string]corev1.Pod
			profileNameIfaceMap map[string][]string
			err                 error
		)

		BeforeEach(func() {
			if configCountsIfce.HA == 0 {
				Skip("Test requires High Availability configuration")
			}

			By("getting original interfaces from profiles")
			profileNameIfaceMap, err = ranptphelper.BuildPtpProfileIfacesMap()
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if configCountsIfce.HA == 0 {
				Skip("Test requires High Availability configuration")
			}

			if ptpConfig.Name == "" {
				Skip("no ptpConfig to restore")
			}

			// remove the resource version from the configuration, so it can be recreated with same values.
			ptpConfig.ObjectMeta.ResourceVersion = ""

			By("restoring deleted configuration")
			_, err = helper.Apiclient.PtpConfigs(parameters.PtpOperatorNamespace).Create(context.Background(),
				&ptpConfig, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("validating all interfaces clock state is LOCKED")
			for _, ptpDaemonPod := range nodeToPtpDaemonPod {
				for _, ifaces := range profileNameIfaceMap {
					ifaceGroupMap := ranptphelper.GetInterfaceGroups(ifaces)
					for ifaceGroup := range ifaceGroupMap {
						err = ranptphelper.WaitForPtpClockStateMetric(ptpDaemonPod, ranptpparameters.LockedState,
							ifaceGroup, 5*time.Minute, 10*time.Second)
						Expect(err).NotTo(HaveOccurred())
					}
				}

				err = ranptphelper.WaitForPtpClockStateMetric(ptpDaemonPod, ranptpparameters.LockedState,
					"", 5*time.Minute, 10*time.Second, "ptp4l")
				Expect(err).NotTo(HaveOccurred())
			}
		})

		// 73095
		It("should change high availability active profile when active profile is deleted",
			polarion.ID("73095"), func() {
				// there is bc ha ptp configuration.
				if configCountsIfce.HA == 0 {
					Skip("Test requires High Availability configuration")
				}

				nodeToPtpDaemonPod, err = ranptphelper.NodesToPtpDaemonPods()
				Expect(err).NotTo(HaveOccurred())

				for nodeName, ptpDaemonPod := range nodeToPtpDaemonPod {
					By("validating only one active profile exists")
					statusProfilesMap, err := ranptphelper.BuildStatusProfileNamesMap(ptpDaemonPod)
					Expect(err).NotTo(HaveOccurred())
					Expect(len(statusProfilesMap[ranptpparameters.Active])).Should(Equal(1))

					By("getting the active ha profile")
					activeProfile, err := ranptphelper.GetHaProfile(ranptpparameters.Active)
					Expect(err).NotTo(HaveOccurred())

					By("getting the the right configuration from the active profile name")
					profileNameConfigMap, err := ranptphelper.BuildProfileNameConfigMap()
					Expect(err).NotTo(HaveOccurred())

					By("saving the configuration for later use")
					ptpConfig = profileNameConfigMap[activeProfile[0]]

					By("deleting the configuration " + ptpConfig.Name)
					err = helper.Apiclient.PtpConfigs(parameters.PtpOperatorNamespace).Delete(context.Background(),
						ptpConfig.Name, metav1.DeleteOptions{})
					Expect(err).NotTo(HaveOccurred())

					By("validating the active configuration changed")
					err = ranptphelper.WaitForMHaMetricsUpdate(ptpDaemonPod, activeProfile[0], time.Minute*2)
					Expect(err).NotTo(HaveOccurred())

					By("validating only one active profile exists")
					statusProfilesMap, err = ranptphelper.BuildStatusProfileNamesMap(ptpDaemonPod)
					Expect(err).NotTo(HaveOccurred())
					Expect(len(statusProfilesMap[ranptpparameters.Active])).Should(Equal(1))

					activeProfiles, err := ranptphelper.GetHaProfile(ranptpparameters.Active)
					Expect(err).NotTo(HaveOccurred())

					ptpNode, err := ranhelper.GetNodeByName(nodeName)
					Expect(err).NotTo(HaveOccurred())

					By("getting the active interface")
					profileSlaveIfaceMap, err := ranptphelper.BuildProfileSlaveInterfaceMap(*ptpNode)
					Expect(err).NotTo(HaveOccurred())

					By("validating active clock state is LOCKED")
					activeIface := profileSlaveIfaceMap[activeProfiles[0]]
					err = ranptphelper.WaitForPtpClockStateMetric(ptpDaemonPod, ranptpparameters.LockedState,
						activeIface, 3*time.Minute, 10*time.Second)
					Expect(err).NotTo(HaveOccurred())
				}
			})
	})

	Context("ordinary clock 2 port failure", func() {
		var (
			ptpDaemonPod      *corev1.Pod
			oc2PortIfaces     []string
			oc2PortIfaceGroup string
		)

		BeforeEach(func() {
			if !ranhelper.IsVersionStringInRange(ranptpparameters.PtpVersion, "4.18", "") {
				Skip("Test is valid from version 4.19")
			}

			if configCountsIfce.OCTwoPort == 0 {
				Skip("Test requires ordinary clock 2 port configuration")
			}

			By("Get the Ordinary Clock interfaces as defined by the configuration")
			configs, err := helper.Apiclient.PtpConfigs(parameters.PtpOperatorNamespace).List(context.Background(),
				metav1.ListOptions{})
			Expect(err).NotTo(HaveOccurred())

			oc2PortConfigs, err := ranptphelper.GetOc2PortPtpConfigs(*configs)
			Expect(err).NotTo(HaveOccurred())

			oc2PortConfig := oc2PortConfigs[0]

			profilesIfacesMap, err := ranptphelper.BuildPtpProfileIfacesMap()
			Expect(err).NotTo(HaveOccurred())

			By("Get a PTP daemon which matches the OC 2 port profile")
			nodePtpProfileMap, err := ranptphelper.GetPtpProfilesPerNode(*oc2PortConfig)
			Expect(err).NotTo(HaveOccurred())

			nodePtpPodMap, err := ranptphelper.NodesToPtpDaemonPods()
			Expect(err).NotTo(HaveOccurred())
			for nodeName, ptpProfiles := range nodePtpProfileMap {
				for _, ptpProfile := range ptpProfiles {
					if ranptphelper.IsOrdinaryClock2PortProfile(ptpProfile) {
						pod := nodePtpPodMap[nodeName]
						ptpDaemonPod = &pod

						oc2PortIfaces = profilesIfacesMap[*ptpProfile.Name]
						Expect(len(oc2PortIfaces)).To(Equal(2))

						oc2PortIfaceGroup = ranptphelper.GetNic(oc2PortIfaces[0])

						break
					}
				}

				if ptpDaemonPod != nil {
					break
				}
			}
			Expect(ptpDaemonPod).ToNot(BeNil())

			By("Validate the Active interface is SLAVE & Passive interface is LISTENING")
			validateSortOc2PortIfaces(*ptpDaemonPod, oc2PortIfaces)
		})

		// 80963
		It("verifies 2-port oc ha failover when active port goes down",
			polarion.ID("80963"), func() {
				By("Bringing down the initial active interface")

				timeIfaceDown := time.Now()

				err := ranptphelper.SetInterfaceStatus(ptpDaemonPod, parameters.PtpContainerName, oc2PortIfaces[0],
					ranptpparameters.Off, 2)
				Expect(err).NotTo(HaveOccurred())

				By("Validate after failover: clock state remains LOCKED, iface roles SLAVE & FAULTY")
				metricsFilter := ranptphelper.NewMetricVector(
					ranptphelper.NewMetricSample(
						ranptpparameters.OpenshiftPtpClockState,
						float64(ranptpparameters.LockedState),
						map[string]string{
							"process": ranptpparameters.ProcessPTP4L,
						},
					),
					ranptphelper.NewMetricSample(
						ranptpparameters.OpenshiftPtpClockState,
						float64(ranptpparameters.LockedState),
						map[string]string{
							"process": ranptpparameters.ProcessPHC2SYS,
						},
					),
					ranptphelper.NewMetricSample(
						ranptpparameters.OpenshiftPtpInterfaceRole,
						float64(ranptpparameters.SlaveRole),
						map[string]string{
							"iface": oc2PortIfaces[1],
						},
					),
					ranptphelper.NewMetricSample(
						ranptpparameters.OpenshiftPtpInterfaceRole,
						float64(ranptpparameters.FaultyRole),
						map[string]string{
							"iface": oc2PortIfaces[0],
						},
					),
				)
				err = ranptphelper.WaitForDesiredMetricVector(*ptpDaemonPod, 1*time.Minute, metricsFilter)
				Expect(err).NotTo(HaveOccurred())

				By("Validate no clock state FREERUN events")
				err = ranptphelper.WaitForEvent(
					ptpDaemonPod, ranptpparameters.CloudEventContainer,
					ranptpparameters.EventTypePtpStateChange,
					ranptpparameters.EventFreeRun, oc2PortIfaceGroup, "/master", timeIfaceDown, 30*time.Second)
				Expect(err).ToNot(BeNil(),
					"No FREERUN event is received")

				restoreValidateOc2Port(*ptpDaemonPod, oc2PortIfaces)
			})

		// 80964
		It("verifies 2-port oc ha holdover & freerun when both ports go down",
			polarion.ID("80964"), func() {
				By("Bringing both interfaces down")

				timeIfaceDown := time.Now()

				for _, iface := range oc2PortIfaces {
					err := ranptphelper.SetInterfaceStatus(ptpDaemonPod, parameters.PtpContainerName, iface,
						ranptpparameters.Off, 2)
					Expect(err).NotTo(HaveOccurred())
				}

				By("Validate after dual failure: clock state FREERUN, iface roles FAULTY")
				metricsFilter := ranptphelper.NewMetricVector(
					ranptphelper.NewMetricSample(
						ranptpparameters.OpenshiftPtpInterfaceRole,
						float64(ranptpparameters.FaultyRole),
						map[string]string{
							"iface": oc2PortIfaces[0],
						},
					),
					ranptphelper.NewMetricSample(
						ranptpparameters.OpenshiftPtpInterfaceRole,
						float64(ranptpparameters.FaultyRole),
						map[string]string{
							"iface": oc2PortIfaces[1],
						},
					),
					ranptphelper.NewMetricSample(
						ranptpparameters.OpenshiftPtpClockState,
						float64(ranptpparameters.FreeRunState),
						map[string]string{
							"process": ranptpparameters.ProcessPTP4L,
						},
					),
					ranptphelper.NewMetricSample(
						ranptpparameters.OpenshiftPtpClockState,
						float64(ranptpparameters.FreeRunState),
						map[string]string{
							"process": ranptpparameters.ProcessPHC2SYS,
						},
					),
				)
				err := ranptphelper.WaitForDesiredMetricVector(*ptpDaemonPod, 5*time.Minute, metricsFilter)
				Expect(err).NotTo(HaveOccurred())

				By("Validate clock state HOLDOVER event")
				err = ranptphelper.WaitForEvent(
					ptpDaemonPod, ranptpparameters.CloudEventContainer,
					ranptpparameters.EventTypePtpStateChange,
					ranptpparameters.EventHoldOver, oc2PortIfaceGroup, "/master", timeIfaceDown, 30*time.Second)
				Expect(err).NotTo(HaveOccurred())

				By("Validate clock state FREERUN event")
				err = ranptphelper.WaitForEvent(
					ptpDaemonPod, ranptpparameters.CloudEventContainer,
					ranptpparameters.EventTypePtpStateChange,
					ranptpparameters.EventFreeRun, oc2PortIfaceGroup, "/master", timeIfaceDown, 30*time.Second)
				Expect(err).NotTo(HaveOccurred())

				restoreValidateOc2Port(*ptpDaemonPod, oc2PortIfaces)
			})

		// 82012
		It("verifies 2-port oc ha passive interface recovery",
			polarion.ID("82012"), func() {
				By("Bringing down the passive interface")

				timeIfaceDown := time.Now()

				err := ranptphelper.SetInterfaceStatus(ptpDaemonPod, parameters.PtpContainerName, oc2PortIfaces[1],
					ranptpparameters.Off, 2)
				Expect(err).NotTo(HaveOccurred())

				By("Validate after failer: clock state remains LOCKED, iface roles SLAVE & FAULTY")
				metricsFilter := ranptphelper.NewMetricVector(
					ranptphelper.NewMetricSample(
						ranptpparameters.OpenshiftPtpClockState,
						float64(ranptpparameters.LockedState),
						map[string]string{
							"process": ranptpparameters.ProcessPTP4L,
						},
					),
					ranptphelper.NewMetricSample(
						ranptpparameters.OpenshiftPtpClockState,
						float64(ranptpparameters.LockedState),
						map[string]string{
							"process": ranptpparameters.ProcessPHC2SYS,
						},
					),
					ranptphelper.NewMetricSample(
						ranptpparameters.OpenshiftPtpInterfaceRole,
						float64(ranptpparameters.SlaveRole),
						map[string]string{
							"iface": oc2PortIfaces[0],
						},
					),
					ranptphelper.NewMetricSample(
						ranptpparameters.OpenshiftPtpInterfaceRole,
						float64(ranptpparameters.FaultyRole),
						map[string]string{
							"iface": oc2PortIfaces[1],
						},
					),
				)
				err = ranptphelper.WaitForDesiredMetricVector(*ptpDaemonPod, 1*time.Minute, metricsFilter)
				Expect(err).NotTo(HaveOccurred())

				By("Validate no clock state HOLDOVER events")
				err = ranptphelper.WaitForEvent(
					ptpDaemonPod, ranptpparameters.CloudEventContainer,
					ranptpparameters.EventTypePtpStateChange,
					ranptpparameters.EventHoldOver, oc2PortIfaceGroup, "/master", timeIfaceDown, 30*time.Second)
				Expect(err).ToNot(BeNil(),
					"No HOLDOVER event is received")

				restoreValidateOc2Port(*ptpDaemonPod, oc2PortIfaces)
			})
	})
})

func restorePtpInterfaces() {
	log.Println("bring up all slave and master interfaces on all ptp pods")

	nodeToPtpDaemonPod, err := ranptphelper.NodesToPtpDaemonPods()
	Expect(err).ToNot(HaveOccurred())

	for nodeName, ptpPod := range nodeToPtpDaemonPod {
		ptpNode, err := ranhelper.GetNodeByName(nodeName)
		Expect(err).NotTo(HaveOccurred())

		slaveIfaces, err := ranptphelper.GetInterfaces(ptpv1.Slave, *ptpNode)
		Expect(err).NotTo(HaveOccurred())
		masterIfaces, err := ranptphelper.GetInterfaces(ptpv1.Master, *ptpNode)
		Expect(err).NotTo(HaveOccurred())

		for _, iface := range slaveIfaces {
			err = ranptphelper.SetInterfaceStatus(&ptpPod, parameters.PtpContainerName, iface,
				ranptpparameters.On, 2)
			Expect(err).NotTo(HaveOccurred())
		}

		// Best effort to bring up master interface as they can be disconnected
		for _, iface := range masterIfaces {
			_ = ranptphelper.SetInterfaceStatus(&ptpPod, parameters.PtpContainerName, iface,
				ranptpparameters.On, 1)
		}
	}
}

// get the ptp daemon pod that runs on the same node as the consumer.
func getPtpDaemonPods(nodeName string) (*corev1.PodList, error) {
	listOptions := metav1.ListOptions{
		LabelSelector: parameters.PtpDaemonsetLabelSelector,
		FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeName),
	}

	ptpDaemonPods, err := helper.Apiclient.Pods(parameters.PtpOperatorNamespace).List(context.Background(),
		listOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to get ptp daemon pods on node: %s", nodeName)
	}

	return ptpDaemonPods, nil
}

func validatePublisherService(nodeName string) {
	serviceName := fmt.Sprintf("ptp-event-publisher-service-%s", nodeName)

	_, err := helper.Apiclient.Services(parameters.PtpOperatorNamespace).Get(context.Background(),
		serviceName, metav1.GetOptions{})
	Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("Service %s is not running in namespace %s: %v",
		serviceName, parameters.PtpOperatorNamespace, err))
}

// Create events by disabling and enabling a slave interface and verify events are being received
// by the specific pod/container.
func verifyEventsAndMetricsSlaveInterfaceDownUp(node *corev1.Node, ptpPod *corev1.Pod, pod *corev1.Pod,
	container string, ifaces []string, skipMetricCheck bool) {
	startTime := time.Now()
	time.Sleep(5 * time.Second) // sleep for 5 seconds to reduce chance of race condition.

	By(fmt.Sprintf("Bring down ptp slave interfaces %v on node %s\n", ifaces, node.Name))

	for _, i := range ifaces {
		err := ranptphelper.SetInterfaceStatus(ptpPod, parameters.PtpContainerName, i,
			ranptpparameters.Off, 2)
		Expect(err).NotTo(HaveOccurred())
	}

	slaveInterface := ifaces[0]
	By(fmt.Sprintf("Wait for ptp [HOLDOVER] state change event after salve interfaces %v goes down", slaveInterface))
	err := ranptphelper.WaitForEvent(pod, container,
		"event.sync.ptp-status.ptp-state-change",
		ranptpparameters.EventHoldOver, slaveInterface, "/master", startTime, 3*time.Minute)
	Expect(err).NotTo(HaveOccurred())

	By(fmt.Sprintf("Wait for event [FREERUN] after slave interfaces "+
		"%s goes down on node %s", slaveInterface, node.Name))

	timeout, err := ranptphelper.GetHoldOverTimeout()
	Expect(err).NotTo(HaveOccurred())

	timeout += time.Since(startTime) + 120*time.Second
	err = ranptphelper.WaitForEvent(pod, container,
		"event.sync.ptp-status.ptp-state-change",
		ranptpparameters.EventFreeRun, slaveInterface, "/master", startTime, timeout)
	Expect(err).NotTo(HaveOccurred())

	if !skipMetricCheck {
		// metrics check
		By("Validate slave interface is in [FREERUN] in ptp metrics")

		err = ranptphelper.WaitForPtpClockStateMetric(*ptpPod, ranptpparameters.FreeRunState,
			slaveInterface, 5*time.Minute, 10*time.Second)
		Expect(err).NotTo(HaveOccurred())
	}

	By(fmt.Sprintf("Bring up ptp slave interfaces %v on node %s\n", ifaces, node.Name))

	for _, i := range ifaces {
		err = ranptphelper.SetInterfaceStatus(ptpPod, parameters.PtpContainerName, i,
			ranptpparameters.On, 2)
		Expect(err).NotTo(HaveOccurred())
	}

	By(fmt.Sprintf("Wait for ptp event [LOCKED] state for all PTP clocks after slave "+
		"interfaces %v goes up on node %s", slaveInterface, node.Name))

	err = ranptphelper.WaitForEvent(pod, container,
		"event.sync.ptp-status.ptp-state-change",
		ranptpparameters.EventLocked, slaveInterface, "/master", startTime, 5*time.Minute)
	Expect(err).NotTo(HaveOccurred())

	if !skipMetricCheck {
		// metrics check
		By("Validate all interfaces are in LOCKED state in ptp metrics")

		err = ranptphelper.WaitForPtpClockStateMetric(*ptpPod, ranptpparameters.LockedState,
			"", 1*time.Minute, 10*time.Second)
		Expect(err).NotTo(HaveOccurred())
	}
}

// restoreValidateOc2Port restores OC 2 port & validates that it's in in active / passive state.
func restoreValidateOc2Port(
	ptpDaemonPod corev1.Pod,
	ifaces []string) {
	GinkgoHelper()

	By("Turn back on the interfaces")
	restorePtpInterfaces()

	By("Validate the Active interface is SLAVE & Passive interface is LISTENING")
	validateSortOc2PortIfaces(ptpDaemonPod, ifaces)
}

// validateSortOc2PortIfaces validates OC 2 port is in active / passive.
// sorts the slice so the first element is active.
func validateSortOc2PortIfaces(
	ptpDaemonPod corev1.Pod,
	ifaces []string) {
	GinkgoHelper()

	Expect(len(ifaces)).To(Equal(2))

	ifaceRoles, err := ranptphelper.GetPodInterfaceRoles(ptpDaemonPod, ifaces)
	Expect(err).NotTo(HaveOccurred())

	// isActivePassive returns true if the iface role value matches an active passive state.
	isActivePassive := func(
		roleA, roleB ranptpparameters.InterfaceRole) bool {
		return roleA == ranptpparameters.SlaveRole &&
			roleB == ranptpparameters.ListeningRole
	}

	ifaceXname, ifaceXRole := ifaces[0], ifaceRoles[ifaces[0]]
	ifaceYname, ifaceYRole := ifaces[1], ifaceRoles[ifaces[1]]

	switch {
	case isActivePassive(ifaceXRole, ifaceYRole):
		// do nothing since the ifaces slice already sorted
	case isActivePassive(ifaceYRole, ifaceXRole):
		ifaces[0] = ifaceYname
		ifaces[1] = ifaceXname
	default:
		Fail(fmt.Sprintf("couldn't deduce OC active / passive interfaces: %s: %d, %s: %d",
			ifaceXname, ifaceXRole, ifaceYname, ifaceYRole))
	}
}
