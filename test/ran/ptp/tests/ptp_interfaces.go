package tests

import (
	"fmt"
	"log"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ptpv1 "github.com/openshift/ptp-operator/api/v1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptphelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptpparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
)

var _ = Describe("PTP Events and Metrics - interface down", func() {
	var (
		errBeforeAll  error
		bcConfigCount int
		// isOcConfigured  bool
		originPtpConfigSpecs = map[string]ptpv1.PtpConfigSpec{}
	)

	execute.BeforeAll(func() {
		var ptpConfigCounts []int
		originPtpConfigSpecs, ptpConfigCounts, errBeforeAll = ptpPretestValidations()
		bcConfigCount = ptpConfigCounts[2]
	})

	BeforeEach(func() {
		// Any failure in execute.BeforeAll only fails the first test case.
		if errBeforeAll != nil {
			Skip(fmt.Sprintf("Error encountered before the first test started: %v", errBeforeAll.Error()))
		}

		// Ensure ptp clocks are locked before starting any test
		err := checkPtpLockState(5*time.Second, 0)
		if err != nil {
			Skip("PTP clocks are not in locked states")
		}
	})

	AfterEach(func() {
		restorePtpInterfaces()
		// Always restore ptpconfigs to original values after each test
		log.Println("Restore ptpconfigs to original specs")
		restorePtpConfigs(originPtpConfigSpecs)
		log.Println("Check ptp clocks are in sync")
		err := checkPtpLockState(5*time.Minute, 10*time.Second)
		Expect(err).ToNot(HaveOccurred())
	})

	// 49743
	It("should generate events when slave interface goes down and up", func() {

		nodeToPtpDaemonPod, err := ranptphelper.NodesToPtpDaemonPods()
		Expect(err).NotTo(HaveOccurred())

		for nodeName, ptpDaemonPod := range nodeToPtpDaemonPod {
			ptpNode, err := ranhelper.GetNodeByName(nodeName)
			Expect(err).NotTo(HaveOccurred())

			interfaces, err := ranptphelper.GetInterfaces(ptpv1.Slave, *ptpNode)
			ifaceGroups := ranptphelper.GetInterfaceGroups(interfaces)
			Expect(err).NotTo(HaveOccurred())

			for _, ifaces := range ifaceGroups {
				time.Sleep(3 * time.Second)
				startTime := time.Now()

				By(fmt.Sprintf("Bring down ptp slave interfaces %v on node %s\n", ifaces, ptpNode.Name))
				for _, i := range ifaces {
					err = ranptphelper.SetInterfaceStatus(&ptpDaemonPod, parameters.PtpContainerName, i,
						ranptpparameters.Off)
					Expect(err).NotTo(HaveOccurred())
				}

				iface := ifaces[0]
				By(fmt.Sprintf("Wait for ptp [HOLDOVER] state change event after salve interfaces %v goes down", ifaces))
				err = ranptphelper.WaitForEvent(&ptpDaemonPod, "event.sync.ptp-status.ptp-state-change",
					ranptpparameters.EventHoldOver, iface, time.Since(startTime), 1*time.Minute)
				Expect(err).NotTo(HaveOccurred())

				By(fmt.Sprintf("Wait for event [FREERUN] after salve interfaces "+
					"%s goes down on node %s", ifaces, ptpNode.Name))
				timeout, err := ranptphelper.GetHoldOverTimeout()
				Expect(err).NotTo(HaveOccurred())
				timeout += time.Since(startTime) + 120*time.Second
				log.Printf("wait for thershold holdover to pass %s\n", timeout)
				err = ranptphelper.WaitForEvent(&ptpDaemonPod, "event.sync.ptp-status.ptp-state-change",
					ranptpparameters.EventFreeRun, iface, time.Since(startTime), timeout)
				Expect(err).NotTo(HaveOccurred())

				// metrics check
				By("Validate slave interface is in [FREERUN] in ptp metrics")
				err = ranptphelper.WaitForPtpClockStateMetric(ptpDaemonPod, ranptpparameters.FreeRunState,
					iface, 2*time.Minute, 10*time.Second)
				Expect(err).NotTo(HaveOccurred())

				startTime = time.Now()
				By(fmt.Sprintf("Bring up ptp slave interfaces %v on node %s\n", ifaces, ptpNode.Name))
				for _, i := range ifaces {
					err = ranptphelper.SetInterfaceStatus(&ptpDaemonPod, parameters.PtpContainerName, i,
						ranptpparameters.On)
					Expect(err).NotTo(HaveOccurred())
				}

				By(fmt.Sprintf("Wait for ptp event [LOCKED] state for all PTP clocks after salve "+
					"interfaces %v goes up on node %s", ifaces, ptpNode.Name))
				err = ranptphelper.WaitForEvent(&ptpDaemonPod, "event.sync.ptp-status.ptp-state-change",
					ranptpparameters.EventLocked, iface, time.Since(startTime), 5*time.Minute)
				Expect(err).NotTo(HaveOccurred())

				// metrics check
				By("Validate all interfaces are in LOCKED state in ptp metrics")
				err = ranptphelper.WaitForPtpClockStateMetric(ptpDaemonPod, ranptpparameters.LockedState,
					"", 1*time.Minute, 10*time.Second)
				Expect(err).NotTo(HaveOccurred())
			}
			// Tests only 1 node
			break
		}
	})

	// 49734
	It("should have no effect when Boundary Clock master interface goes down and up", func() {
		if bcConfigCount == 0 {
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
				By(fmt.Sprintf("Bring down ptp Boundary Clock master interfaces %v on node %s\n", ifaces, ptpNode.Name))
				for _, i := range ifaces {
					err = ranptphelper.SetInterfaceStatus(&ptpDaemonPod, parameters.PtpContainerName, i,
						ranptpparameters.Off)
					Expect(err).NotTo(HaveOccurred())
				}

				By("Validate PTP metric stays in locked after bringing down BC master interface")
				err = ranptphelper.WaitForPtpClockStateMetric(ptpDaemonPod, ranptpparameters.LockedState,
					iface, 45*time.Second, 30*time.Second)
				Expect(err).NotTo(HaveOccurred(), "PTP metric did not stay in locked state after "+
					"bringing down BC master interface")

				By(fmt.Sprintf("Validate NO [HOLDOVER] state transition event is generated after master "+
					"interface  %s goes down on node %s", iface, ptpNode.Name))
				err = ranptphelper.WaitForEvent(&ptpDaemonPod, "event.sync.ptp-status.ptp-state-change",
					ranptpparameters.EventHoldOver, iface, time.Since(startTime), 1*time.Minute)
				Expect(err).To(HaveOccurred(), "event received for clock state change to "+
					"[HOLDOVER] after bringing down BC master interface")

				By(fmt.Sprintf("Bring up ptp Boundary Clock master interfaces %v on node %s\n", ifaces, ptpNode.Name))
				for _, i := range ifaces {
					err = ranptphelper.SetInterfaceStatus(&ptpDaemonPod, parameters.PtpContainerName, i,
						ranptpparameters.On)
					Expect(err).NotTo(HaveOccurred())
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
})

func restorePtpInterfaces() {
	log.Println("bring up all slave and master interfaces on all ptp pods")

	nodeToPtpDaemonPod, err := ranptphelper.NodesToPtpDaemonPods()
	Expect(err).ToNot(HaveOccurred())

	for nodeName, ptpPod := range nodeToPtpDaemonPod {
		var allIfaces []string

		ptpNode, err := ranhelper.GetNodeByName(nodeName)
		Expect(err).NotTo(HaveOccurred())

		slaveIfaces, err := ranptphelper.GetInterfaces(ptpv1.Slave, *ptpNode)
		Expect(err).NotTo(HaveOccurred())
		masterIfaces, err := ranptphelper.GetInterfaces(ptpv1.Master, *ptpNode)
		Expect(err).NotTo(HaveOccurred())
		allIfaces = append(allIfaces, slaveIfaces...)
		allIfaces = append(allIfaces, masterIfaces...)

		for _, iface := range allIfaces {
			err = ranptphelper.SetInterfaceStatus(&ptpPod,
				parameters.PtpContainerName,
				iface,
				ranptpparameters.On)
			Expect(err).NotTo(HaveOccurred())
		}
	}
}
