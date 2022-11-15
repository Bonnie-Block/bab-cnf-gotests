package ranhweventhelper

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"

	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/Masterminds/sprig"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/k8snetworkplumbingwg/sriov-network-operator/pkg/render"
	"github.com/noirbizarre/gonja"
	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/pkg/errors"
	bmerv1alpha1 "github.com/redhat-cne/hw-event-proxy-operator/api/v1alpha1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhwevent/ranhweventhelper/rfclient"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhwevent/ranhweventparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	podUtil "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// Snipit from the logs
// time="2022-02-14T07:41:12Z" level=info msg="Latency for the event: 2 ms\n"
// time="2022-02-14T07:41:12Z" level=debug msg="received event {\"id\":\"240cfb3b-c1f4-47b2-baea-1c94147663a9\",
// \"type\":\"event.redfish.alert\",\"source\":\"/cluster/node/helix28.lab.eng.tlv2.redhat.com/redfish/event\",
// \"dataContentType\":\"application/json\",\"time\":\"2022-02-14T07:41:12.628Z\",\"data\":{\"version\":\"v1\",
// \"values\":[{\"resource\":\"/redfish/v1/Systems\",\"dataType\":\"notification\",\"valueType\":\"redfish-event\",
// \"value\":{\"@odata.context\":\"/redfish/v1/$metadata#Event.Event\",\"Context\":\"root\",
// \"@odata.type\":\"#Event.v1_5_0.Event\",\"Events\":[{\"Context\":\"root\",\"EventGroupID\":0,\"EventID\":\"2177\",
// \"EventTimestamp\":\"2022-02-14T07:41:17+0200\",\"Message\":\"The system board fail-safe current is less than the
// lower critical threshold.\",\"MessageArgs\":[\"fail-safe\"],\"Severity\":\"Critical\",\"MessageID\":\"AMP0301\",
// \"MemberID\":\"32743\",\"EventType\":\"Alert\"}],\"ID\":\"a4a003fc-8a51-11ec-85d7-b07b25e354f8\",
// \"Name\":\"Event Array\"}}]}}"

// getMsgIDType1 parse dell version 1.0 events messages.
func getMsgIDType1(eventJSON string) ([]timestampEventType, error) {
	var (
		events    []timestampEventType
		eventType rfclient.EventType
	)

	eventUnquote := strings.ReplaceAll(eventJSON, "\\", "")
	err := json.Unmarshal([]byte(eventUnquote), &eventType)

	if err != nil {
		log.Printf("Error when parsing message ID: %s", err)
	} else if len(eventType.Data.Data.Events) > 0 {
		events = append(events, timestampEventType{
			eventType.Time, eventType.Data.Data.Events[0].MessageID})

	}

	return events, err
}

// IsEventJSON returns a json in case it is a valid json or empty string if not.
func IsEventJSON(line string) string {
	r := regexp.MustCompile(`[received event]\{(.*)\}`)

	eventJSON := r.FindString(line)

	if eventJSON == "" {
		return ""
	}

	return eventJSON
}

// getMsgIDType15 parse dell version 1.5 events messages.
func getMsgIDType15(eventJSON string) ([]timestampEventType, error) {
	var (
		e15    rfclient.EventType15
		events []timestampEventType
	)

	eventUnquote := strings.ReplaceAll(eventJSON, "\\", "")
	err := json.Unmarshal([]byte(eventUnquote), &e15)

	if err != nil {
		log.Printf("Error when parsing message ID: %s", err)

		return events, err
	}

	for _, value := range e15.Data.Values {
		for _, event := range value.Value.Events {
			events = append(events, timestampEventType{event.EventTimestamp, event.MessageID})
		}
	}

	return events, nil
}

// GetMsgID returns the message ID from the event that was received.
func GetMsgID(eventJSON string) ([]timestampEventType, error) {
	events, err := getMsgIDType1(eventJSON)
	if err != nil || len(events) == 0 {
		events, err = getMsgIDType15(eventJSON)
	}

	return events, err
}

// Contains checks if a string appears in a array.
func Contains(arr []string, str string) bool {
	for _, a := range arr {
		if a == str {
			return true
		}
	}

	return false
}

type timestampEventType struct {
	EventTimestamp, MessageID string
}

// GetHTTPS retry GET on HTTPS target until OK is received.
func GetHTTPS(url string) error {
	retryClient := retryablehttp.NewClient()
	retryClient.HTTPClient.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	retryClient.RetryMax = 10
	retryClient.RetryWaitMin = 30 * time.Second

	resp, err := retryClient.Get(url) //nolint:noctx

	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("response code was %v, we expected to get 200 OK", resp.StatusCode)
	}

	return nil
}

// GetAppRoute uses ranhweventparameters to query the application exposed path.
func GetAppRoute() (string, error) {
	routeList, err := helper.Apiclient.Routes(parameters.BmerOperatorNamespace).List(context.Background(),
		metav1.ListOptions{})
	if err != nil {
		return "", err
	}

	for _, route := range routeList.Items {
		if route.Name == ranhweventparameters.AppRouteName {
			routePath := "https://" + route.Spec.Host + "/webhook"

			return routePath, nil
		}

		log.Print("GetAppRoute() host: " + route.Spec.Host)
	}

	return "", fmt.Errorf("failed to find route for app: %v in namespace: %v",
		ranhweventparameters.AppRouteName, parameters.BmerOperatorNamespace)
}

// GetConsumers get all consumer pods that are deployed.
func GetConsumers() (*corev1.PodList, error) {
	consumerPods, err := helper.Apiclient.Pods(parameters.BmerOperatorNamespace).List(context.Background(),
		metav1.ListOptions{
			LabelSelector: ranhweventparameters.ConsumerPodLabel})

	if err != nil {
		return nil, fmt.Errorf("failed to get consumer pods from namespace: %v due to: %w",
			parameters.BmerOperatorNamespace, err)
	}

	if ranhweventparameters.DebugTest {
		for _, pod := range consumerPods.Items {
			log.Print("found consumer pod: " + pod.Name)
		}
	}

	return consumerPods, nil
}

// GetDeployImages query the ClusterServiceVersions CRD used to deploy the app for the mirrored images to use in deploy.
func GetDeployImages() (map[string]string, error) {
	var images = make(map[string]string)

	csvs, err := helper.Apiclient.ClusterServiceVersions(parameters.BmerOperatorNamespace).List(
		context.TODO(), metav1.ListOptions{})
	if err != nil {
		return images, fmt.Errorf("failed to query ClusterServiceVersions at namespace: %v named: %v due to: %w",
			parameters.BmerOperatorNamespace, ranhweventparameters.HwEventCsv, err)
	}

	var csv v1alpha1.ClusterServiceVersion
	for _, csv = range csvs.Items {
		if strings.HasPrefix(csv.Name, ranhweventparameters.HwEventCsv) {
			break
		}
	}
	// the CSV starts with "bare-metal-event-relay" but ends with specific version .4.10.0-202208150436
	if !strings.HasPrefix(csv.Name, ranhweventparameters.HwEventCsv) {
		return images, fmt.Errorf(
			"failed to find a ClusterServiceVersions starting with %v", ranhweventparameters.HwEventCsv)
	}

	for _, relatedImage := range csv.Spec.RelatedImages {
		// open shift 4.10 and 4.11 images differ, so I am checking for both
		for imageRole, imageOptions := range ranhweventparameters.RequiredImages {
			for _, imageOption := range imageOptions {
				if strings.HasPrefix(relatedImage.Name, imageOption) {
					log.Printf("Found %v image: %v\n", imageRole, relatedImage.Image)

					images[imageRole] = relatedImage.Image
				}
			}
		}
	}

	if len(getMapKeys(ranhweventparameters.RequiredImages)) > len(images) {
		for imageRole := range ranhweventparameters.RequiredImages {
			_, exist := images[imageRole]
			if !exist {
				return images, fmt.Errorf("image for %v was not found in CSV: %v", imageRole, csv.Name)
			}
		}
	}

	images[ranhweventparameters.ConsumerImageName] = helper.Config.Ran.HwEventConsumerImage

	return images, nil
}

func getTemplatePath() (string, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	// this should be ranhwevent and parent is ran dir
	parentDir := filepath.Dir(currentDir)

	return parentDir + string(os.PathSeparator), nil
}

// GetConsumerManifest renders a jinja template with mirrored images locations.
func GetConsumerManifest(images map[string]string, transportType string) (string, error) {
	if len(images) < 3 {
		return "", fmt.Errorf("GetConsumerManifest() failed due to less images then expected: %v", images)
	}

	templatePath, err := getTemplatePath()
	if err != nil {
		return "", fmt.Errorf("failed to get template path due to: %w", err)
	}

	var manifestPath string

	if transportType == ranhweventparameters.TransportHTTP {
		manifestPath = ranhweventparameters.ConsumerManifestHTTP
	} else if transportType == ranhweventparameters.TransportAMQP {
		manifestPath = ranhweventparameters.ConsumerManifestAMQP
	}

	template, err := gonja.FromFile(templatePath + manifestPath)
	if err != nil {
		return "", err
	}

	return template.Execute(gonja.Context{
		"kube_rbac_proxy_image":                images["kube_rbac_proxy_image"],
		"cloud_event_proxy_image":              images["cloud_event_proxy_image"],
		ranhweventparameters.ConsumerImageName: images[ranhweventparameters.ConsumerImageName],
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
func DeployConsumers(mirroredImages map[string]string, transportType string) error {
	consumers, err := GetConsumers()

	if err == nil && len(consumers.Items) > 0 {
		return fmt.Errorf("consumers already deployed in cluster. skipping creating them")
	}

	// consumer pod needs the mirrored images
	err = deployConsumerPod(mirroredImages, transportType)
	if err != nil {
		return fmt.Errorf("failed to create consumer pod due to: %w", err)
	}

	var retryCounter int

	err = wait.PollImmediate(5*time.Second, 5*time.Minute, func() (bool, error) {
		deployment, err := helper.Apiclient.Deployments(parameters.BmerOperatorNamespace).Get(
			context.Background(),
			ranhweventparameters.ConsumerDeploymentName,
			metav1.GetOptions{},
		)
		if err != nil {
			log.Printf("failed to update container 0 image from %v,"+
				" will retry\n", helper.Config.Ran.HwEventConsumerImage)

			return false, nil
		}

		// update consumer image source to point to disconnected repository.
		if deployment.Spec.Template.Spec.Containers[0].Image != helper.Config.Ran.HwEventConsumerImage {
			// Update dummy image to configured value
			deployment.Spec.Template.Spec.Containers[0].Image = helper.Config.Ran.HwEventConsumerImage
			_, err = helper.Apiclient.Deployments(parameters.BmerOperatorNamespace).Update(
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
				helper.Config.Ran.HwEventConsumerImage)
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

// ConfigHwEventProxyObjects create various hw event proxy cluster objects to allow application to start working.
func ConfigHwEventProxyObjects() error {
	_, err := os.Stat(helper.Config.Ran.HwEventConfigsDir)

	if os.IsNotExist(err) {
		pwd, _ := os.Getwd()

		return fmt.Errorf("failed to find directory: %v in current path: %v",
			helper.Config.Ran.HwEventConfigsDir, pwd)
	}

	_, err = helper.Apiclient.Deployments(parameters.BmerOperatorNamespace).Get(
		context.Background(),
		ranhweventparameters.ConsumerDeploymentName,
		metav1.GetOptions{},
	)

	if err != nil {
		err = ranhelper.ApplyObjects(helper.Config.Ran.HwEventConfigsDir)
	} else {
		err = ranhelper.UpdateObjects(helper.Config.Ran.HwEventConfigsDir)
	}

	if err != nil {
		return fmt.Errorf("failed to deploy application config due to: %w", err)
	}

	return nil
}

func deployConsumerPod(mirroredImages map[string]string, transportType string) error {
	manifest, err := GetConsumerManifest(mirroredImages, transportType)

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

// DestroyConsumers uses parameters from ranhweventparameters to destroy the consumer setup.
// it returns a map of errors encountered during deletion of the setup.
func DestroyConsumers() (destroyErrors []error) {
	deployment, err := helper.Apiclient.Deployments(parameters.BmerOperatorNamespace).Get(
		context.Background(), ranhweventparameters.ConsumerDeploymentName, metav1.GetOptions{})

	if err == nil {
		err = helper.Apiclient.Client.Delete(context.TODO(), deployment)
		if err != nil {
			destroyErrors = append(destroyErrors,
				fmt.Errorf("failed to delete consumer deployment: %v due to: %w", deployment, err))
		}
	} else {
		destroyErrors = append(destroyErrors, fmt.Errorf("failed to query consumer deployment due to: %w", err))
	}

	err = ranhelper.DeleteObjects(helper.Config.Ran.HwEventConfigsDir)

	if err != nil {
		destroyErrors = append(destroyErrors,
			fmt.Errorf("failed to destroy using manifest files read from: %v due to %w",
				helper.Config.Ran.HwEventConfigsDir, err))
	}

	err = wait.PollImmediate(5*time.Second, 5*time.Minute, func() (bool, error) {
		_, err := helper.Apiclient.Deployments(parameters.BmerOperatorNamespace).Get(
			context.Background(),
			ranhweventparameters.ConsumerDeploymentName,
			metav1.GetOptions{},
		)
		if err != nil {
			return true, nil
		}

		return false, nil
	})

	if err != nil {
		destroyErrors = append(destroyErrors, fmt.Errorf("failed to destroy consumer deployemnt: %v due to: %w",
			ranhweventparameters.ConsumerDeploymentName, err))
	}

	return destroyErrors
}

// GetWorkerNode get the node that supplies the redfish for this test.
func GetWorkerNode() (*corev1.Node, error) {
	workers, err := nodes.GetByRole(helper.Apiclient, parameters.RoleWorker)
	if err != nil {
		log.Printf("Error when attempting to get worker node due to %v\n", err)

		return nil, err
	}

	return &workers[0], nil
}

// PurgeNamespace remove pods and namespace for a given namespace.
func PurgeNamespace(namespace string, clientSet *client.ClientSet) error {
	err := namespaces.CleanPods(namespace, clientSet)
	if err != nil {
		log.Printf("Failed to purge pods in namespace: %v due to: %v\n", namespace, err)

		return err
	}

	err = namespaces.DeleteAndWait(clientSet, parameters.PrivPodNamespace, 5*time.Minute)

	if err != nil {
		log.Printf("Failed to remove namespace: %v due to: %v\n",
			namespace, err)

		return err
	}

	return nil
}

// PurgePrivPodNamespace Remove privileged pods used to get the node vendor.
func PurgePrivPodNamespace() error {
	log.Println("Remove privileged pods used to get the node vendor")

	return PurgeNamespace(parameters.PrivPodNamespace, helper.Apiclient)
}

// RestartPod deletes a pod marked by a given label
// it then waits for a given timeout for the pod to resume running state.
func RestartPod(label string, timeout time.Duration) error {
	pod, err := GetPodByLabel(label)

	if err != nil {
		return err
	}

	podUID := pod.UID
	log.Printf("Deleting pod %v ...", pod.Name)
	err = helper.Apiclient.Pods(pod.Namespace).Delete(context.Background(), pod.Name, metav1.DeleteOptions{})

	if err != nil {
		return fmt.Errorf("failed to delete pod named: %v due to: %w", pod.Name, err)
	}

	err = wait.PollImmediate(5*time.Second, timeout, func() (bool, error) {
		pod, err := GetPodByLabel(label)
		if err != nil {
			return false, err
		}
		if pod.UID == podUID {
			return false, nil
		}
		if pod.Status.Phase == corev1.PodRunning {
			for _, c := range pod.Status.ContainerStatuses {
				if !c.Ready {
					return false, nil
				}
			}
			log.Printf("Pod %v recovered", pod.Name)

			return true, nil
		}

		return false, nil
	})

	if err != nil {
		return fmt.Errorf("failed to restart pod named %v due to: %w", pod.Name, err)
	}

	return nil
}

// RestartSidecar kills the sidecar container in hw-event-proxy pod
// then waits for a given timeout for the container to be back to Ready.
func RestartSidecar(label string, timeout time.Duration) error {
	pod, err := GetPodByLabel(label)

	if err != nil {
		return err
	}

	var restartCount int32

	for _, c := range pod.Status.ContainerStatuses {
		if isCloudEventSidecar(c.Name) {
			restartCount = c.RestartCount

			break
		}
	}

	for _, c := range pod.Spec.Containers {
		if isCloudEventSidecar(c.Name) {
			log.Printf("Killing container %v ...", c.Name)
			buffer, err := podUtil.ExecCommand(helper.Apiclient, pod, []string{"/bin/sh", "-c", "kill 1"}, c.Name)

			if err != nil {
				return fmt.Errorf("fail to kill sidecar %w: %s", err, buffer.String())
			}

			break
		}
	}

	err = wait.PollImmediate(5*time.Second, timeout, func() (bool, error) {
		// repolling pod object to get the latest status
		pod, err := GetPodByLabel(label)
		if err != nil {
			return false, err
		}
		for _, c := range pod.Status.ContainerStatuses {
			if isCloudEventSidecar(c.Name) {
				if (c.RestartCount > restartCount) && c.Ready {
					log.Printf("Container %v recovered, restart count %v -> %v", c.Name, restartCount, c.RestartCount)

					return true, nil
				}

				return false, nil
			}
		}

		return false, nil
	})

	if err != nil {
		return fmt.Errorf("failed to restart container named %v due to: %w", pod.Name, err)
	}

	return nil
}

// GetPodByLabel get all pods in the parameters.BmerOperatorNamespace
// that o have a given label.
func GetPodByLabel(label string) (corev1.Pod, error) {
	Pods, err := helper.Apiclient.Pods(parameters.BmerOperatorNamespace).List(context.Background(),
		metav1.ListOptions{
			LabelSelector: label})

	if err != nil {
		return corev1.Pod{}, err
	}

	pod := Pods.Items[0]

	return pod, nil
}

func isHwEventSecret(namespace, secretName string) bool {
	secret, err := helper.Apiclient.Secrets(namespace).Get(context.Background(), secretName, metav1.GetOptions{})

	return err == nil && len(secret.Data) > 0
}

// CreateHwEventSecret sets the redfish credentials using a kubernetes secret.
func CreateHwEventSecret(secretName, namespace, hostname, username, password string) error {
	if isHwEventSecret(namespace, secretName) {
		return fmt.Errorf("HwEvent secret already exist. skip creating it")
	}

	secret := corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "apps/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"username": []byte(username),
			"password": []byte(password),
		},
		StringData: map[string]string{
			"hostaddr": hostname,
		},
		Type: "Opaque",
	}

	secretOut, err := helper.Apiclient.Secrets(namespace).Create(context.Background(), &secret, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create %v secret due to: %w", secretName, err)
	}

	log.Printf("Created secret: %v\n", secretOut.Name)

	return nil
}

// DeleteHwEventSecret Delete the secret created to start the feature app.
func DeleteHwEventSecret(namespace, secretName string) error {
	if !isHwEventSecret(namespace, secretName) {
		return fmt.Errorf("missing HwEvent secret. will not delete it")
	}

	err := helper.Apiclient.Secrets(namespace).Delete(context.Background(), secretName, metav1.DeleteOptions{})

	return err
}

// CheckCustomResourceDefinition validates the CostumeResourceDefinition of the feature exist.
func CheckCustomResourceDefinition() error {
	hwEventList := &bmerv1alpha1.HardwareEventList{}
	err := helper.Apiclient.List(context.TODO(), hwEventList)

	if err != nil {
		return fmt.Errorf("failed to list hw event proxy custom resource definition due to: %w", err)
	}

	for _, result := range hwEventList.Items {
		if result.Name == ranhweventparameters.CustomResourceDefinition {
			return nil
		}
	}

	return fmt.Errorf("failed to find custum resource definition: %v in cluster",
		ranhweventparameters.CustomResourceDefinition)
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
		if isCloudEventSidecar(c.Name) {
			for _, a := range c.Args {
				if strings.Contains(a, "transport-host=http") {
					return ranhweventparameters.TransportHTTP, nil
				} else if strings.Contains(a, "transport-host=amqp") {
					return ranhweventparameters.TransportAMQP, nil
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

func isCloudEventSidecar(name string) bool {
	if (name == "cloud-event-proxy") || (name == "cloud-event-sidecar") {
		return true
	}

	return false
}
