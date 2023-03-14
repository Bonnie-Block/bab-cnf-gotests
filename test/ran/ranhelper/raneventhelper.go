package ranhelper

import (
	"bytes"
	"context"
	"fmt"

	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/Masterminds/sprig"
	"github.com/k8snetworkplumbingwg/sriov-network-operator/pkg/render"
	"github.com/noirbizarre/gonja"
	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/pkg/errors"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// GetConsumers get all consumer pods that are deployed in a namespace.
func GetConsumers(namespace string) (*corev1.PodList, error) {
	consumerPods, err := helper.Apiclient.Pods(namespace).List(context.Background(),
		metav1.ListOptions{
			LabelSelector: ranparameters.ConsumerPodLabel})

	if err != nil {
		return nil, fmt.Errorf("failed to get consumer pods from namespace: %v due to: %w",
			parameters.BmerNamespace, err)
	}

	if ranparameters.DebugTest {
		for _, pod := range consumerPods.Items {
			log.Print("found consumer pod: " + pod.Name)
		}
	}

	return consumerPods, nil
}

// GetDeployImages query the ClusterServiceVersions CRD used to deploy the app for the mirrored images to use in deploy.
func GetDeployImages(namespace string) (map[string]string, error) {
	var images = make(map[string]string)

	csvs, err := helper.Apiclient.ClusterServiceVersions(namespace).List(
		context.TODO(), metav1.ListOptions{})
	if err != nil {
		return images, fmt.Errorf("failed to query ClusterServiceVersions at namespace: %v named: %v due to: %w",
			namespace, ranparameters.CsvDict()(namespace), err)
	}

	var csv v1alpha1.ClusterServiceVersion
	for _, csv = range csvs.Items {
		if strings.HasPrefix(csv.Name, ranparameters.CsvDict()(namespace)) {
			break
		}
	}
	// the CSV starts with "bare-metal-event-relay" but ends with specific version .4.10.0-202208150436
	if !strings.HasPrefix(csv.Name, ranparameters.CsvDict()(namespace)) {
		return images, fmt.Errorf(
			"failed to find a ClusterServiceVersions starting with %v", ranparameters.CsvDict()(namespace))
	}

	for _, relatedImage := range csv.Spec.RelatedImages {
		// open shift 4.10 and 4.11 images differ, so I am checking for both
		for imageRole, imageOptions := range ranparameters.RequiredImages {
			for _, imageOption := range imageOptions {
				if strings.HasPrefix(relatedImage.Name, imageOption) {
					log.Printf("Found %v image: %v\n", imageRole, relatedImage.Image)

					images[imageRole] = relatedImage.Image
				}
			}
		}
	}

	if len(getMapKeys(ranparameters.RequiredImages)) > len(images) {
		for imageRole := range ranparameters.RequiredImages {
			_, exist := images[imageRole]
			if !exist {
				return images, fmt.Errorf("image for %v was not found in CSV: %v", imageRole, csv.Name)
			}
		}
	}

	images[ranparameters.ConsumerImageName] = helper.Config.Ran.BmerConsumerImage

	return images, nil
}

func getTemplatePath() (string, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	// this should be bmer and parent is ran dir
	parentDir := filepath.Dir(currentDir)

	return parentDir + string(os.PathSeparator), nil
}

// GetConsumerManifest renders a jinja template with mirrored images locations.
func GetConsumerManifest(images map[string]string, transportType string, namespace string) (string, error) {
	if len(images) < 3 {
		return "", fmt.Errorf("GetConsumerManifest() failed due to less images then expected: %v", images)
	}

	templatePath, err := getTemplatePath()
	if err != nil {
		return "", fmt.Errorf("failed to get template path due to: %w", err)
	}

	var manifestPath string

	if transportType == ranparameters.TransportHTTP {
		manifestPath = ranparameters.TemplatePathDict()(namespace) + "/" + ranparameters.ConsumerManifestHTTP
	} else if transportType == ranparameters.TransportAMQP {
		manifestPath = ranparameters.TemplatePathDict()(namespace) + "/" + ranparameters.ConsumerManifestAMQP
	}

	template, err := gonja.FromFile(templatePath + manifestPath)
	if err != nil {
		return "", err
	}
	// need to get the node name here
	return template.Execute(gonja.Context{
		"kube_rbac_proxy_image":         images["kube_rbac_proxy_image"],
		"cloud_event_proxy_image":       images["cloud_event_proxy_image"],
		ranparameters.ConsumerImageName: images[ranparameters.ConsumerImageName],
	})
}

// getMapValueOrFallback returns the value of inputMap[key] if it exists, fallback otherwise.
// As a special case, it also returns fallback if the value of m[key] is
// the empty string.
func getMapValueOrFallback(inputMap map[string]interface{}, key, fallback string) interface{} {
	val, valid := inputMap[key]
	if !valid {
		return fallback
	}

	s, valid := val.(string)
	if valid && s == "" {
		return fallback
	}

	return val
}

// isMapValueSet returns the value of inputMap[key] if key exists, otherwise false
// Different from getOr because it will return zero values.
func isMapValueSet(inputMap map[string]interface{}, key string) interface{} {
	val, ok := inputMap[key]
	if !ok {
		return false
	}

	return val
}

func renderTemplate(manifest string, renderData *render.RenderData) (*bytes.Buffer, error) {
	tmpl := template.New("template").Option("missingkey=error")
	if renderData.Funcs != nil {
		tmpl.Funcs(renderData.Funcs)
	}

	// Add universal functions
	tmpl.Funcs(template.FuncMap{"getMapValueOrFallback": getMapValueOrFallback, "isMapValueSet": isMapValueSet})
	tmpl.Funcs(sprig.TxtFuncMap())

	if _, err := tmpl.Parse(manifest); err != nil {
		return nil, errors.Wrapf(err, "failed to parse manifest %s as template", manifest)
	}

	rendered := bytes.Buffer{}
	if err := tmpl.Execute(&rendered, renderData.Data); err != nil {
		return nil, errors.Wrapf(err, "failed to render manifest %s", manifest)
	}

	return &rendered, nil
}

// DeployConsumers renders templates from yaml files and creates these objects in the cluster
// it also returns the consumer manifest that is created using mirrored images.
func DeployConsumers(mirroredImages map[string]string, transportType string, namespace string) error {
	consumers, err := GetConsumers(namespace)

	if err == nil && len(consumers.Items) > 0 {
		return fmt.Errorf("consumers already deployed in cluster. skipping creating them")
	}

	// consumer pod needs the mirrored images
	err = deployConsumerPod(mirroredImages, transportType, namespace)
	if err != nil {
		return fmt.Errorf("failed to create consumer pod due to: %w", err)
	}

	var retryCounter int

	err = wait.PollImmediate(5*time.Second, 5*time.Minute, func() (bool, error) {
		deployment, err := helper.Apiclient.Deployments(namespace).Get(
			context.Background(),
			ranparameters.ConsumerDeploymentName,
			metav1.GetOptions{},
		)
		if err != nil {
			log.Printf("failed to update container 0 image from %v,"+
				" will retry\n", helper.Config.Ran.BmerConsumerImage)

			return false, nil
		}

		// update consumer image source to point to disconnected repository.
		if deployment.Spec.Template.Spec.Containers[0].Image != helper.Config.Ran.BmerConsumerImage {
			// Update dummy image to configured value
			deployment.Spec.Template.Spec.Containers[0].Image = helper.Config.Ran.BmerConsumerImage
			_, err = helper.Apiclient.Deployments(parameters.BmerNamespace).Update(
				context.Background(),
				deployment,
				metav1.UpdateOptions{},
			)
			if err != nil {
				return false, err
			}
			log.Printf("image updated,"+
				" from %v -> %v retrieve updated deployment in next round\n",
				deployment.Spec.Template.Spec.Containers[0].Image,
				helper.Config.Ran.BmerConsumerImage)
		}

		if deployment.Status.ReadyReplicas > 0 &&
			deployment.Status.ReadyReplicas >= deployment.Status.AvailableReplicas {
			log.Printf("consumer deployment is done due to ready replica: %v/%v after %v retries\n ",
				deployment.Status.ReadyReplicas, deployment.Status.AvailableReplicas,
				retryCounter+1)

			return true, nil
		}

		return false, nil
	})

	if err != nil {
		return fmt.Errorf("failed to Deploy consumers due to: %w", err)
	}

	return nil
}

func deployConsumerPod(mirroredImages map[string]string, transportType string, namespace string) error {
	manifest, err := GetConsumerManifest(mirroredImages, transportType, namespace)

	if err != nil {
		return err
	}

	data := render.MakeRenderData()
	rendered, err := renderTemplate(manifest, &data)

	if err != nil {
		return fmt.Errorf("failed to render consumer template due to: %w", err)
	}

	decoder := yaml.NewYAMLOrJSONDecoder(rendered, 4096)

	var manifestPayload unstructured.Unstructured

	err = wait.PollImmediate(1*time.Second, 5*time.Minute, func() (bool, error) {
		err = decoder.Decode(&manifestPayload)

		if err != nil {
			if errors.Is(err, io.EOF) {
				return true, nil
			}

			return false, fmt.Errorf("failed to unmarshal manifest due to: %w", err)
		}

		return false, nil
	})
	if err != nil {
		return fmt.Errorf("failed deployConsumerPod() during decoding manifest payload, due to: %w", err)
	}

	err = helper.Apiclient.Client.Create(context.TODO(), &manifestPayload)

	if err != nil {
		log.Printf("failed to create consumer deployment due to: %v manifest used is below:\n", err)
		log.Println(manifestPayload)

		return fmt.Errorf("failed to create consumer deployment due to: %w", err)
	}

	return nil
}

// DestroyConsumers uses parameters from bmerparameters to destroy the consumer setup.
// it returns a map of errors encountered during deletion of the setup.
func DestroyConsumers() (destroyErrors []error) {
	deployment, err := helper.Apiclient.Deployments(parameters.BmerNamespace).Get(
		context.Background(), ranparameters.ConsumerDeploymentName, metav1.GetOptions{})

	if err == nil {
		err = helper.Apiclient.Client.Delete(context.TODO(), deployment)
		if err != nil {
			destroyErrors = append(destroyErrors,
				fmt.Errorf("failed to delete consumer deployment: %v due to: %w", deployment, err))
		}
	} else {
		destroyErrors = append(destroyErrors, fmt.Errorf("failed to query consumer deployment due to: %w", err))
	}

	err = DeleteObjects(helper.Config.Ran.BmerConfigsDir)

	if err != nil {
		destroyErrors = append(destroyErrors,
			fmt.Errorf("failed to destroy using manifest files read from: %v due to %w",
				helper.Config.Ran.BmerConfigsDir, err))
	}

	err = wait.PollImmediate(5*time.Second, 5*time.Minute, func() (bool, error) {
		_, err := helper.Apiclient.Deployments(parameters.BmerNamespace).Get(
			context.Background(),
			ranparameters.ConsumerDeploymentName,
			metav1.GetOptions{},
		)
		if err != nil {
			return true, nil
		}

		return false, nil
	})

	if err != nil {
		destroyErrors = append(destroyErrors, fmt.Errorf("failed to destroy consumer deployemnt: %v due to: %w",
			ranparameters.ConsumerDeploymentName, err))
	}

	return destroyErrors
}

// WaitForDeploymentReady wait for a deployment to reach ready state or timeout at 5 Min.
// returns bool for if ready and error.
func WaitForDeploymentReady(client *client.ClientSet, namespace, deployment string) (err error) {
	err = wait.PollImmediate(5*time.Second, 5*time.Minute, func() (bool, error) {
		ready, err := helper.IsDeploymentReady(client, namespace, deployment)
		if err == nil && ready {
			return true, nil
		}

		return false, nil
	})

	return err
}

// GetTransportType ...
func GetTransportType(client *client.ClientSet, namespace, deployment string) (transportType string, err error) {
	d, err := client.Deployments(namespace).Get(context.Background(), deployment, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	for _, c := range d.Spec.Template.Spec.Containers {
		if IsCloudEventSidecar(c.Name) {
			for _, a := range c.Args {
				if strings.Contains(a, "transport-host=http") {
					return ranparameters.TransportHTTP, nil
				} else if strings.Contains(a, "transport-host=amqp") {
					return ranparameters.TransportAMQP, nil
				}
			}
		}
	}

	return "", nil
}

func getMapKeys(input map[string][]string) []string {
	output := make([]string, 0, len(input))
	for key := range input {
		output = append(output, key)
	}

	return output
}

func IsCloudEventSidecar(name string) bool {
	if (name == "cloud-event-proxy") || (name == "cloud-event-sidecar") {
		return true
	}

	return false
}
