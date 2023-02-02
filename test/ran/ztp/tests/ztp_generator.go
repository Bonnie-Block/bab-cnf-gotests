package tests

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ztp/ranztphelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ztp/ranztpparameters"
)

var _ = Describe("ZTP Generator Tests", Label("ztp-generator"), func() {

	var siteConfigPath string

	BeforeEach(func() {
		// On automation / QE the user will be "kni"
		output, err := helper.ExecAndLogCommand(true, 1*time.Minute, "whoami")
		Expect(err).ToNot(HaveOccurred())

		// Cleanup the output by removing whitespaces
		user := strings.TrimSpace(string(output))

		// Ensure it is not an empty string
		Expect(user).ToNot(Equal(""))

		// Build the path
		siteConfigPath = "/home/" + user + "/site-configs"

		if _, err := os.Stat(siteConfigPath); err != nil {
			Skip(fmt.Sprintf("could not find site config repo at '%s', unable to continue without repo", siteConfigPath))
		}
	})

	Context("using the site generator image", func() {
		It("generate install time crs, manifests, and policies and verify they are present", func() {
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
				// Validate the result - There should be 12 files in the output directory
				files, err := os.ReadDir(fmt.Sprintf("%s/siteconfig/out/generated_installCRs/site-plan-helix49/", siteConfigPath))
				Expect(err).ToNot(HaveOccurred())
				Expect(len(files)).To(Equal(10))
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
				var files []fs.DirEntry
				var err error

				// There should be 3 files in this directory
				files, err = os.ReadDir(fmt.Sprintf("%s/policygentemplates/out/generated_configCRs", siteConfigPath))
				Expect(err).ToNot(HaveOccurred())
				Expect(len(files)).To(Equal(4))

				// There should be 4 files in this directory
				files, err = os.ReadDir(fmt.Sprintf("%s/policygentemplates/out/generated_configCRs/common", siteConfigPath))
				Expect(err).ToNot(HaveOccurred())
				Expect(len(files)).To(Equal(4))

				// There should be 3 files in this directory
				files, err = os.ReadDir(fmt.Sprintf("%s/policygentemplates/out/generated_configCRs/group-du-sno", siteConfigPath))
				Expect(err).ToNot(HaveOccurred())
				Expect(len(files)).To(Equal(3))

				// There should be 3 files in this directory
				files, err = os.ReadDir(fmt.Sprintf("%s/policygentemplates/out/generated_configCRs/helix49", siteConfigPath))
				Expect(err).ToNot(HaveOccurred())
				Expect(len(files)).To(Equal(3))
			})
		})
	})
})
