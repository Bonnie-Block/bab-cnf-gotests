package tests

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ztp/ranztphelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ztp/ranztpparameters"
	testClient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/machineconfigpool"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/polarion"
)

const (
	// This directory houses the YAML files necessary for the ztp argocd node delete test.
	addAnnotationDirectory  = "ztp-test/node-delete/add-annotation"
	addSuppressionDirectory = "ztp-test/node-delete/add-suppression"

	// Bare Metal Object information.
	crAnnotationKey = "bmac.agent-install.openshift.io/remove-agent-and-node-on-delete"
)

var _ = Describe("ZTP Argocd node delete Tests", polarion.ID("72463"), Label("ztp-argocd-node-delete"), func() {
	// These tests use the hub and spoke
	var clusterList []*testClient.ClientSet
	var plusOneNodeName string
	var bmhNamespace string

	BeforeEach(func() {
		// Initialize cluster list
		clusterList = ranztphelper.GetAllTestClients()

		// Check that the required clusters are present
		err := ranhelper.IsClustersPresent(clusterList)
		if err != nil {
			Skip(fmt.Sprintf("error occurred validating required clusters are present: %s", err.Error()))
		}
		// Check for minimum ztp version
		By("Checking the ZTP version", func() {
			if !ranhelper.IsVersionStringInRange(ranztphelper.ZtpVersion, "4.16", "") {
				Skip(fmt.Sprintf(
					"unable to run test on ztp version '%s' as it is less than minimum '%s",
					ranztphelper.ZtpVersion,
					"4.16",
				))
			}
		})

		By("Check that the 'worker' mcp is ready", func() {
			// Get the worker mcp
			ready, err := machineconfigpool.IsMcpReady(ranztphelper.GetSpokeClient(), "worker")
			Expect(err).ToNot(HaveOccurred())

			if !ready {
				Skip("worker mcp is not ready")
			}
		})

		// Check that the cluster contains both a master/control-plane and worker node
		By("Checking that the cluster contains both a master/control-plane and worker node", func() {
			// At this point, we can assume the cluster is an SNO+1 setup and we need to gather the
			// name of the plus one worker node.

			// First, check if there are two total nodes in the cluster
			if !nodes.IsSNOPlusOneWorkerCluster(ranztphelper.GetSpokeClient()) {
				Skip("cluster does not contain a single master and worker node")
			}

			// Get the name of the plus one worker node
			// If the worker is not ready, this will return an empty string
			plusOneNodeName = nodes.GetPlusOneWorkerNodeName(ranztphelper.GetSpokeClient())
			Expect(plusOneNodeName).ToNot(BeEmpty())

			// Get the namespace of the plus one worker node
			bmhNamespace, err = ranztphelper.GetBmhNamespace(plusOneNodeName)
			Expect(err).ToNot(HaveOccurred())

			// Check that the plus one worker node is not empty
			Expect(bmhNamespace).ToNot(BeEmpty())
		})
	})

	AfterEach(func() {
		// Reset the clusters app back to default after each test

		// This is essentially our "return-to-normal" test case.
		By("Resetting the clusters app back to the original settings", func() {
			err := ranztphelper.SetGitDetailsInArcgocd(
				ranztphelper.ArgocdApps[ranztpparameters.ArgocdClustersAppName].Repo,
				ranztphelper.ArgocdApps[ranztpparameters.ArgocdClustersAppName].Branch,
				ranztphelper.ArgocdApps[ranztpparameters.ArgocdClustersAppName].Path,
				ranztpparameters.ArgocdClustersAppName,
				true,
				true,
			)
			Expect(err).ToNot(HaveOccurred())
		})

		// Check that the cluster is back to SNO+1
		By("Checking that the cluster is back to SNO+1", func() {
			// Check that the cluster contains both a master/control-plane and worker node
			// At this point, we can assume the cluster is an SNO+1 setup and we need to gather the
			// name of the plus one worker node.

			// Wait for the number of workers in the cluster to be 2
			err := nodes.WaitForNumberOfNodes(ranztphelper.GetSpokeClient(), 2, time.Minute*45)
			Expect(err).ToNot(HaveOccurred())

			// Assert that the cluster is an SNO+1 setup
			Expect(nodes.IsSNOPlusOneWorkerCluster(ranztphelper.GetSpokeClient())).To(BeTrue())
		})
	})

	It("should delete a worker node from the cluster", func() {
		// https://issues.redhat.com/browse/CNF-6299
		// The ztp test data is stored in a nested directory within the ztp repo
		testGitPath := ranztphelper.JoinGitPaths(
			[]string{
				ranztphelper.ArgocdApps[ranztpparameters.ArgocdClustersAppName].Path,
				addAnnotationDirectory,
			},
		)

		By("Checking if the add-annotation git path exists", func() {
			if !ranztphelper.DoesGitPathExist(
				ranztphelper.ArgocdApps[ranztpparameters.ArgocdClustersAppName].Repo,
				ranztphelper.ArgocdApps[ranztpparameters.ArgocdClustersAppName].Branch,
				testGitPath+"/kustomization.yaml",
			) {
				Skip(fmt.Sprintf("git path '%s' could not be found", testGitPath))
			}
		})

		By("updating the argo app to apply the crAnnotation", func() {
			// Update the Argo app to point to the new test kustomization
			err := ranztphelper.SetGitDetailsInArcgocd(
				ranztphelper.ArgocdApps[ranztpparameters.ArgocdClustersAppName].Repo,
				ranztphelper.ArgocdApps[ranztpparameters.ArgocdClustersAppName].Branch,
				testGitPath,
				ranztpparameters.ArgocdClustersAppName,
				true,
				true,
			)
			Expect(err).ToNot(HaveOccurred())
		})

		By("Wait for the crAnnotation to be added to the worker node", func() {
			/*
					nodes:
				- hostname: node6
					role: "worker"
					crAnnotations:
						add:
							BareMetalHost:
								bmac.agent-install.openshift.io/remove-agent-and-node-on-delete: true
			*/

			// Wait for 5 minutes for the annotation to be applied to the bmh object
			err := ranztphelper.WaitForBareMetalHostAnnotation(plusOneNodeName, bmhNamespace, crAnnotationKey, time.Minute*30)
			Expect(err).To(BeNil())
		})

		// Set the git path to the suppression directory
		testGitPath = ranztphelper.JoinGitPaths(
			[]string{
				ranztphelper.ArgocdApps[ranztpparameters.ArgocdClustersAppName].Path,
				addSuppressionDirectory,
			},
		)

		By("Checking if the suppression git path exists", func() {
			if !ranztphelper.DoesGitPathExist(
				ranztphelper.ArgocdApps[ranztpparameters.ArgocdClustersAppName].Repo,
				ranztphelper.ArgocdApps[ranztpparameters.ArgocdClustersAppName].Branch,
				testGitPath+"/kustomization.yaml",
			) {
				Skip(fmt.Sprintf("git path '%s' could not be found", testGitPath))
			}
		})

		By("updating the argo app to apply the suppression to the spec", func() {
			// Update the Argo app to point to the new test kustomization
			err := ranztphelper.SetGitDetailsInArcgocd(
				ranztphelper.ArgocdApps[ranztpparameters.ArgocdClustersAppName].Repo,
				ranztphelper.ArgocdApps[ranztpparameters.ArgocdClustersAppName].Branch,
				testGitPath,
				ranztpparameters.ArgocdClustersAppName,
				true,
				false,
			)
			Expect(err).ToNot(HaveOccurred())
		})

		By("Wait for the worker node to be removed", func() {
			// Wait for 'agent' and the 'bmh' objects to be removed.
			err := ranztphelper.WaitForBareMetalHostDeprovisioning(plusOneNodeName, bmhNamespace, time.Minute*60)
			Expect(err).To(BeNil())

			// Wait for the node to be removed from the spoke cluster
			err = ranztphelper.WaitForNodeDeletion(plusOneNodeName, time.Minute*60)
			Expect(err).To(BeNil())
		})

		// Check that the cluster is healthy
		// The cluster should be healthy after the node is removed

		By("Check that the cluster is healthy", func() {
			// Check that the cluster is healthy
			result, err := cluster.IsClusterStable(ranztphelper.GetSpokeClient())
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(BeTrue())
		})
	})
})
