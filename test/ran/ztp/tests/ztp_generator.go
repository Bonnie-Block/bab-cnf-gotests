package tests

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ztp/ranztphelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ztp/ranztpparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/polarion"
	"gopkg.in/yaml.v3"
)

var _ = Describe("ZTP Generator Tests", Label("ztp-generator"), func() {

	var siteConfigPath string
	var user string

	BeforeEach(func() {
		// On automation / QE the user will be "kni"
		output, err := helper.ExecAndLogCommand(true, 1*time.Minute, "whoami")
		Expect(err).ToNot(HaveOccurred())

		// Cleanup the output by removing whitespaces
		user = strings.TrimSpace(string(output))

		// Ensure it is not an empty string
		Expect(user).ToNot(Equal(""))

		// Build the path
		siteConfigPath = "/home/" + user + "/site-configs"

		if _, err := os.Stat(siteConfigPath); err != nil {
			Skip(fmt.Sprintf("could not find site config repo at '%s', unable to continue without repo", siteConfigPath))
		}

		// Check for minimum ztp version
		By("Checking the ZTP version", func() {
			if ranztphelper.ZtpVersion != "" && !ranhelper.IsVersionStringInRange(
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

	AfterEach(func() {
		// Cleanup the generated artifacts
		By("Deleting the generated manifests and policies", func() {
			var err error
			_, err = helper.ExecAndLogCommand(true, 1*time.Minute, "sudo", "rm", "-rf", siteConfigPath+"/siteconfig/out")
			Expect(err).ToNot(HaveOccurred())
			_, err = helper.ExecAndLogCommand(true, 1*time.Minute, "sudo", "rm", "-rf", siteConfigPath+"/policygentemplates/out")
			Expect(err).ToNot(HaveOccurred())
		})
	})

	// 54355
	Context("using the site generator image", func() {
		It("generate install time crs, manifests, and policies and verify they are present", polarion.ID("54355"), func() {
			// https://issues.redhat.com/browse/CNF-6302

			// This will be used to store the tag of the ztp site generate container
			var ztpImageTag string

			By("validating the image version for the site generator", func() {
				// Figure out what the latest y stream is and use that
				if ranztphelper.ZtpVersion == "" {
					ztpImageTag = "latest"
				} else {
					// Since brew is a lot faster than skopeo, we want to use it if its available
					brew, _ := helper.ExecAndLogCommand(true, 1*time.Minute, "which", "brew")

					// If brew was not found then the stdout from the command will be empty
					// as the error would be reported in stderr
					if string(brew) == "" {
						// We can use skopeo to check the available tags
						log.Println("Finding image tags using skopeo")

						// Prepare the argument list
						command := "skopeo list-tags " +
							fmt.Sprintf("docker://%s", ranztpparameters.ZtpSiteGenerateImageName) +
							fmt.Sprintf(" | grep %s", ranztphelper.ZtpVersion) +
							" | sort -V" +
							" | tail -1" +
							" | tr -d '\"'" +
							" | tr -d ','"

						// Run the command and get the output
						output, err := helper.ExecAndLogCommand(true, 1*time.Minute, "bash", []string{"-c", command}...)
						Expect(err).ToNot(HaveOccurred())

						// Convert the output to a string and remove any whitespace
						ztpImageTag = strings.TrimSpace(string(output))

					} else {
						// We can use brew to check the available tags
						log.Println("Finding image tags using brew")

						command := "brew list-builds" +
							" --package=ztp-site-generate-container" +
							" --state=COMPLETE" +
							" --quiet" +
							fmt.Sprintf(" | grep %s", ranztphelper.ZtpVersion) +
							" | sort -V" +
							" | tail -1" +
							" | awk '{ print $1 }'"

						// Run the command and get the output
						output, err := helper.ExecAndLogCommand(true, 1*time.Minute, "bash", []string{"-c", command}...)
						Expect(err).ToNot(HaveOccurred())

						// Convert the output to a string and remove any whitespace
						ztpImageTag = strings.TrimSpace(string(output))

						// The starting position is the beginning of the vX.Y.Z section
						pos1 := strings.Index(ztpImageTag, "-v") + 1

						// The tag from brew would also be missing the v so we need to add that here
						ztpImageTag = ztpImageTag[pos1:]
					}
				}
			})

			log.Printf("Detected ztp image tag '%s'\n", ztpImageTag)

			By("generating the install time CRs and manifests", func() {
				// Prepare the argument list
				args := []string{
					"run",
					"--rm",
					"-v",
					fmt.Sprintf("%s/siteconfig/:/resources:Z", siteConfigPath),
					fmt.Sprintf("%s:%s", ranztpparameters.ZtpSiteGenerateImageName, ztpImageTag),
					"generator",
					"install",
					"-E",
					"/resources/",
				}

				// Run the command
				_, err := helper.ExecAndLogCommand(true, 1*time.Minute, "podman", args...)
				Expect(err).ToNot(HaveOccurred())
			})

			By("validating CRs and manifests were created", func() {
				// Validate the result
				installCRsDir := fmt.Sprintf("%s/siteconfig/out/generated_installCRs/", siteConfigPath)
				siteDirs, err := os.ReadDir(installCRsDir)
				Expect(err).ToNot(HaveOccurred())

				for _, dir := range siteDirs {
					files, err := os.ReadDir(installCRsDir + dir.Name())
					Expect(err).ToNot(HaveOccurred())
					Expect(len(files)).To(BeNumerically(">", 9))
				}
			})

			By("generating the policies", func() {
				// Prepare the argument list
				args := []string{
					"run",
					"--rm",
					"-v",
					fmt.Sprintf("%s/policygentemplates/:/resources:Z", siteConfigPath),
					fmt.Sprintf("%s:%s", ranztpparameters.ZtpSiteGenerateImageName, ztpImageTag),
					"generator",
					"config",
					".",
				}

				// Run the command
				_, err := helper.ExecAndLogCommand(true, 1*time.Minute, "podman", args...)
				Expect(err).ToNot(HaveOccurred())
			})

			By("validating the policies were created", func() {
				// Validate the result
				expectedKind := []string{"Policy", "PlacementRule", "PlacementBinding"}

				// Expect to have at least 3 subdirs - common, group du, site
				policyCRsDir := fmt.Sprintf("%s/policygentemplates/out/generated_configCRs/", siteConfigPath)
				configDirs, err := os.ReadDir(policyCRsDir)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(configDirs)).To(BeNumerically(">=", 3))

				for _, dir := range configDirs {
					files, err := os.ReadDir(policyCRsDir + dir.Name())
					Expect(err).ToNot(HaveOccurred())
					Expect(len(files)).To(BeNumerically(">=", 3))

					for _, f := range files {
						fBytes, err := os.ReadFile(policyCRsDir + dir.Name() + "/" + f.Name())
						Expect(err).ToNot(HaveOccurred())

						fcontent := make(map[string]interface{})
						err = yaml.Unmarshal(fBytes, &fcontent)
						Expect(err).ToNot(HaveOccurred())

						kind, ok := fcontent["kind"].(string)
						Expect(ok).Should(BeTrue())
						Expect(kind).Should(BeElementOf(expectedKind))
					}
				}
			})
		})
	})
})
