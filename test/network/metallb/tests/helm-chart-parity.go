package tests

import (
	"context"
	"fmt"

	"time"

	metallboperatorv1beta1 "github.com/metallb/metallb-operator/api/v1beta1"
	metallbutils "github.com/metallb/metallb-operator/test/e2e/metallb"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmetallbhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmlbparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/polarion"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("MetalLB", func() {

	AfterEach(func() {
		By("Should remove Metallb Configuration")
		metallb, err := metallbutils.Get(
			netmlbparameters.MetalLBOperatorNameSpace,
			netmlbparameters.UseMetallbResourcesFromFile,
		)
		Expect(err).ToNot(HaveOccurred())
		metallbutils.Delete(metallb)
	})

	// 54131
	It("Deployment Helm Chart with all parameters set", polarion.ID("54131"), func() {
		By("Setup Metallb")
		createMetalLBHelmChartNoUpdate()

		By("Verify Metallb has been updated")
		err := validateHelmChartDeployment()
		Expect(err).ToNot(HaveOccurred())
	})

	// 54132
	It("Update Helm Chart parameters with baseline deployment", polarion.ID("54132"), func() {
		By("Setup Metallb")
		netmetallbhelper.SetupMetalLB()
		updateMetalLBHelmChart()

		By("Verify Metallb has been updated")
		Eventually(func() error {

			err := validateHelmChartDeployment()

			return err
		}, 1*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())
	})
})

// updateMetalLBHelmChart updates an existing metallb deployment.
func updateMetalLBHelmChart() {
	By("should update MetalLB")

	metallb, err := getMetallb()
	Expect(err).ToNot(HaveOccurred(), "Unable to Get metallb configuration")

	metallbConfig := defineMetalLBHelmChart(metallb)
	Expect(helper.Apiclient.Update(context.Background(), metallbConfig)).Should(Succeed())
}

// createMetalLBHelmChartNoUpdate defines metallb with helm chart parity for deployment with no updates.
func createMetalLBHelmChartNoUpdate() {
	var metallbConfig *metallboperatorv1beta1.MetalLB

	metallb, err := getMetallb()

	if err != nil {
		metallbConfig = defineMetalLBHelmChart(metallb)
	}

	Expect(helper.Apiclient.Create(context.Background(), metallbConfig)).Should(Succeed())
}

// validateHelmChartControllerDeployment validates metallb configuration in deployment.
func validateHelmChartDeployment() error {
	err := validateHelmChartControllerDeployment()

	if err != nil {
		return err
	}

	return validateHelmChartSpeakerDaemonSet()
}

// validateHelmChartControllerDeployment validates metallb configuration in deployment.
func validateHelmChartControllerDeployment() error {
	Eventually(func() error {
		_, err := helper.Apiclient.Deployments(netmlbparameters.MetalLBOperatorNameSpace).Get(context.Background(),
			netmlbparameters.MetalLBDeploymentName, metav1.GetOptions{})

		return err
	}, 30*time.Second, netmlbparameters.Interval).ShouldNot(HaveOccurred(),
		"Unable to get metallb controller deployment")

	controlDep, err := helper.Apiclient.Deployments(netmlbparameters.MetalLBOperatorNameSpace).Get(context.Background(),
		netmlbparameters.MetalLBDeploymentName, metav1.GetOptions{})

	if err != nil {
		return err
	}

	if controlDep.Spec.Template.Spec.PriorityClassName != netmlbparameters.HelmChartKeyExample {
		return fmt.Errorf("controller Spec PriorityClassName is not found")
	}

	if *controlDep.Spec.Template.Spec.RuntimeClassName != netmlbparameters.HelmChartKeyMyClass {
		return fmt.Errorf("controller Spec RuntimeClassName is not found")
	}

	if controlDep.Spec.Template.Annotations[netmlbparameters.MetalLBDeploymentName] !=
		netmlbparameters.HelmChartDemoController {
		return fmt.Errorf("controller Spec Annotation is not found")
	}

	if controlDep.Spec.Template.Spec.Affinity.PodAffinity.
		RequiredDuringSchedulingIgnoredDuringExecution[0].LabelSelector.MatchLabels["component"] !=
		netmlbparameters.HelmChartControllerTest {
		return fmt.Errorf("controller Spec MatchLabels is not found")
	}

	if controlDep.Spec.Template.Spec.Tolerations[0].Key != netmlbparameters.HelmChartKeyExample ||
		controlDep.Spec.Template.Spec.Tolerations[0].Operator != "Exists" ||
		controlDep.Spec.Template.Spec.Tolerations[0].Effect != "NoExecute" {
		return fmt.Errorf("controller Spec Tolerations is not found")
	}

	return nil
}

// validateHelmChartSpeakerDaemonSet valiadates metallab daemanset configuration.
func validateHelmChartSpeakerDaemonSet() error {
	Eventually(func() error {
		_, err := helper.Apiclient.DaemonSets(netmlbparameters.MetalLBOperatorNameSpace).Get(context.Background(),
			netmlbparameters.MetalLBDaemonsetName, metav1.GetOptions{})

		return err
	}, 30*time.Second, netmlbparameters.Interval).ShouldNot(HaveOccurred(),
		"Unable to Get metallb speaker dameonSet")

	speakerDep, err := helper.Apiclient.DaemonSets(netmlbparameters.MetalLBOperatorNameSpace).Get(context.Background(),
		netmlbparameters.MetalLBDaemonsetName, metav1.GetOptions{})
	Expect(err).ToNot(HaveOccurred())

	if speakerDep.Spec.Template.Spec.PriorityClassName != netmlbparameters.HelmChartHighPriority {
		return fmt.Errorf("speaker Spec PriorityClassName is not found")
	}

	if *speakerDep.Spec.Template.Spec.RuntimeClassName != netmlbparameters.HelmChartKeyMyClass {
		return fmt.Errorf("speaker Spec RuntimeClassName is not found")
	}

	if speakerDep.Spec.Template.Annotations[netmlbparameters.MetalLBDeploymentName] !=
		netmlbparameters.HelmChartDemoSpeaker {
		return fmt.Errorf("speaker Spec Annotation is not found")
	}

	if speakerDep.Spec.Template.Spec.Affinity.PodAffinity.
		RequiredDuringSchedulingIgnoredDuringExecution[0].LabelSelector.MatchLabels["component"] !=
		netmlbparameters.HelmChartSpeakerTest {
		return fmt.Errorf("speaker Spec MatchLabels is not found")
	}

	if speakerDep.Spec.Template.Spec.Tolerations[0].Operator != "Exists" {
		return fmt.Errorf("speaker Spec Tolerations is not found")
	}

	return nil
}

// defineMetalLBHelmChart defines a metallb configuration extra fields from helm-chart.
func defineMetalLBHelmChart(metallb *metallboperatorv1beta1.MetalLB) *metallboperatorv1beta1.MetalLB {
	metallb.Spec.ControllerConfig = &metallboperatorv1beta1.Config{PriorityClassName: netmlbparameters.HelmChartKeyExample,
		RuntimeClassName: netmlbparameters.HelmChartKeyMyClass,
		Annotations: map[string]string{
			netmlbparameters.MetalLBDeploymentName: netmlbparameters.HelmChartDemoController}}

	metallb.Spec.ControllerConfig.Affinity = &k8sv1.Affinity{PodAffinity: &k8sv1.PodAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: []k8sv1.PodAffinityTerm{
			{
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"component": netmlbparameters.HelmChartControllerTest,
					},
				},
				TopologyKey: parameters.LabelHostname,
			},
		},
	},
	}

	metallb.Spec.ControllerTolerations = []k8sv1.Toleration{
		{
			Key:      netmlbparameters.HelmChartKeyExample,
			Operator: netmlbparameters.HelmChartKeyOperator,
			Effect:   netmlbparameters.HelmChartKeyEffect,
		},
	}

	metallb.Spec.SpeakerConfig = &metallboperatorv1beta1.Config{PriorityClassName: netmlbparameters.HelmChartHighPriority,
		RuntimeClassName: netmlbparameters.HelmChartKeyMyClass,
		Annotations: map[string]string{
			netmlbparameters.MetalLBDeploymentName: netmlbparameters.HelmChartDemoSpeaker}}

	metallb.Spec.SpeakerConfig.Affinity = &k8sv1.Affinity{PodAffinity: &k8sv1.PodAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: []k8sv1.PodAffinityTerm{
			{
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"component": netmlbparameters.HelmChartSpeakerTest,
					},
				},
				TopologyKey: parameters.LabelHostname,
			},
		},
	},
	}

	metallb.Spec.SpeakerTolerations = []k8sv1.Toleration{
		{
			Key:      netmlbparameters.HelmChartKeyExample,
			Operator: netmlbparameters.HelmChartKeyOperator,
			Effect:   netmlbparameters.HelmChartKeyEffect,
		},
	}

	return metallb
}

// getMetallb Gets metallb configurations and status.
func getMetallb() (*metallboperatorv1beta1.MetalLB, error) {
	metallb, err := metallbutils.Get(
		netmlbparameters.MetalLBOperatorNameSpace,
		netmlbparameters.UseMetallbResourcesFromFile,
	)

	if err != nil {
		return nil, err
	}

	err = helper.Apiclient.Get(context.Background(), runtimeclient.ObjectKey{Namespace: metallb.Namespace,
		Name: metallb.Name}, metallb)

	return metallb, err
}
