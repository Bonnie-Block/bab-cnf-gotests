package tests

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ztp/ranztphelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ztp/ranztpparameters"
	testClient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/polarion"
	"k8s.io/apimachinery/pkg/util/wait"
)

var _ = Describe("ZTP Argocd clusters Tests", Ordered, Label("ztp-argocd-clusters"), func() {

	// These tests use the hub and spoke
	var clusterList []*testClient.ClientSet

	BeforeAll(func() {
		// Initialize cluster list
		clusterList = ranztphelper.GetAllTestClients()
	})

	BeforeEach(func() {
		// Check that the required clusters are present
		err := ranhelper.IsClustersPresent(clusterList)
		if err != nil {
			Skip(fmt.Sprintf("error occurred validating required clusters are present: %s", err.Error()))
		}
		// Check for minimum ztp version
		By("Checking the ZTP version", func() {
			if !ranhelper.IsVersionStringInRange(
				ranztphelper.ZtpVersion,
				"4.11",
				"",
			) {
				Skip(fmt.Sprintf(
					"unable to run test on ztp version '%s' as it is less than minimum '%s",
					ranztphelper.ZtpVersion,
					"4.11",
				))
			}
		})
	})

	// 54238
	Context("override the klusterlet addon configuration", Label("ztp-klusterlet"), func() {
		It("should override the klusterlet addon configuration and verify the change", polarion.ID("54238"), func() {
			// https://issues.redhat.com/browse/CNF-6299
			// The ztp test data is stored in a nested directory within the ztp repo
			testGitPath := ranztphelper.JoinGitPaths(
				[]string{
					ranztphelper.ArgocdApps[ranztpparameters.ArgocdClustersAppName].Path,
					"ztp-test/klusterlet-addon",
				},
			)

			By("Checking if the git path exists", func() {
				if !ranztphelper.DoesGitPathExist(
					ranztphelper.ArgocdApps[ranztpparameters.ArgocdClustersAppName].Repo,
					ranztphelper.ArgocdApps[ranztpparameters.ArgocdClustersAppName].Branch,
					testGitPath+"/kustomization.yaml",
				) {
					Skip(fmt.Sprintf("git path '%s' could not be found", testGitPath))
				}
			})

			By("updating the argo app", func() {
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

			By("Validating the klusterlet addon change occurred", func() {
				// Wait until the kac config gets updated
				err := wait.PollImmediate(
					ranztpparameters.ArgocdChangeInterval,
					ranztpparameters.ArgocdChangeTimeout,
					func() (bool, error) {
						// Get the kac config
						kac, err := ranztphelper.GetKlusterletConfiguration(ranztphelper.SpokeName, ranztphelper.SpokeName)

						// This should never result in an error
						if err != nil {
							return true, err
						}

						return kac.Spec.SearchCollectorConfig.Enabled, nil
					},
				)
				// Get the klusterlet addon configuration
				Expect(err).ToNot(HaveOccurred())
			})
		})

		It("Should not have nmstateconfig cr when nodeNetwork section does not exist on siteConfig", func() {
			testGitPath := ranztphelper.JoinGitPaths(
				[]string{
					ranztphelper.ArgocdApps[ranztpparameters.ArgocdClustersAppName].Path,
					"ztp-test/remove-nmstate",
				},
			)

			By("Checking if the git path exists", func() {
				if !ranztphelper.DoesGitPathExist(
					ranztphelper.ArgocdApps[ranztpparameters.ArgocdClustersAppName].Repo,
					ranztphelper.ArgocdApps[ranztpparameters.ArgocdClustersAppName].Branch,
					testGitPath+"/kustomization.yaml",
				) {
					Skip(fmt.Sprintf("git path '%s' could not be found", testGitPath))
				}
			})

			By("Check nmstateconfig cr exists", func() {
				nmStateConfigList, err := ranztphelper.GetNmStateConfigList()
				Expect(err).ToNot(HaveOccurred())
				Expect(nmStateConfigList.Items).ToNot(BeEmpty(), "No NMstateconfig found before test begins")
			})

			By("Reconfigure clusters app to set the ztp directory to the ztp-tests/remove-nmstate dir", func() {
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

			By("Check nmstate CR is gone under spoke cluster NS on hub", func() {
				nmStateConfigList, err := ranztphelper.GetNmStateConfigList()
				Expect(err).ToNot(HaveOccurred())
				Expect(nmStateConfigList.Items).To(BeEmpty(), "NMstateconfig was found")
			})
		})
	})

	AfterEach(func() {
		// Reset the clusters app back to default after each test
		By("Resetting the clusters app back to the original settings", func() {
			err := ranztphelper.SetGitDetailsInArcgocd(
				ranztphelper.ArgocdApps[ranztpparameters.ArgocdClustersAppName].Repo,
				ranztphelper.ArgocdApps[ranztpparameters.ArgocdClustersAppName].Branch,
				ranztphelper.ArgocdApps[ranztpparameters.ArgocdClustersAppName].Path,
				ranztpparameters.ArgocdClustersAppName,
				true,
				false,
			)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
