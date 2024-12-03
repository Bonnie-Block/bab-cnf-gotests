package ranbmerhelper

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	bmerv1alpha1 "github.com/redhat-cne/hw-event-proxy-operator/api/v1alpha1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/bmer/ranbmerparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	podUtil "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
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

// GetAppRoute uses ranbmerparameters to query the application exposed path.
func GetAppRoute() (string, error) {
	routeList, err := helper.Apiclient.Routes(parameters.BmerNamespace).List(context.Background(),
		metav1.ListOptions{})
	if err != nil {
		return "", err
	}

	for _, route := range routeList.Items {
		if route.Name == ranbmerparameters.AppRouteName {
			routePath := "https://" + route.Spec.Host + "/webhook"

			return routePath, nil
		}

		log.Print("GetAppRoute() host: " + route.Spec.Host)
	}

	return "", fmt.Errorf("failed to find route for app: %v in namespace: %v",
		ranbmerparameters.AppRouteName, parameters.BmerNamespace)
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
	podToRestart, err := GetPodByLabel(label)

	if err != nil {
		return err
	}

	podUID := podToRestart.UID
	log.Printf("Deleting pod %v ...", podToRestart.Name)
	err = helper.Apiclient.Pods(
		podToRestart.Namespace).Delete(context.Background(), podToRestart.Name, metav1.DeleteOptions{})

	if err != nil {
		return fmt.Errorf("failed to delete pod named: %v due to: %w", podToRestart.Name, err)
	}

	err = wait.PollImmediate(5*time.Second, timeout, func() (bool, error) {
		podToRestart, err = GetPodByLabel(label)
		if err != nil {
			return false, nil
		}
		if podToRestart.UID == podUID {
			return false, nil
		}
		if podToRestart.Status.Phase == corev1.PodRunning {
			for _, c := range podToRestart.Status.ContainerStatuses {
				if !c.Ready {
					return false, nil
				}
			}
			log.Printf("Pod %v recovered", podToRestart.Name)

			return true, nil
		}

		return false, nil
	})

	if err != nil {
		return fmt.Errorf("failed to restart pod named %v due to: %w", podToRestart.Name, err)
	}

	return nil
}

// RestartSidecar kills the sidecar container in hw-event-proxy pod
// then waits for a given timeout for the container to be back to Ready.
func RestartSidecar(label string, timeout time.Duration) error {
	podByLabel, err := GetPodByLabel(label)

	if err != nil {
		return err
	}

	var restartCount int32

	for _, c := range podByLabel.Status.ContainerStatuses {
		if ranhelper.IsCloudEventSidecar(c.Name) {
			restartCount = c.RestartCount

			break
		}
	}

	for _, c := range podByLabel.Spec.Containers {
		if ranhelper.IsCloudEventSidecar(c.Name) {
			log.Printf("Killing container %v ...", c.Name)
			buffer, err := podUtil.ExecCommand(helper.Apiclient, podByLabel, []string{"/bin/sh", "-c", "kill 1"}, c.Name)

			if err != nil {
				return fmt.Errorf("fail to kill sidecar %w: %v", err, buffer)
			}

			break
		}
	}

	err = wait.PollImmediate(5*time.Second, timeout, func() (bool, error) {
		// repolling podByLabel object to get the latest status
		podByLabel, err = GetPodByLabel(label)
		if err != nil {
			return false, err
		}
		for _, c := range podByLabel.Status.ContainerStatuses {
			if ranhelper.IsCloudEventSidecar(c.Name) {
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
		return fmt.Errorf("failed to restart container named %v due to: %w", podByLabel.Name, err)
	}

	return nil
}

// GetPodByLabel get all pods in the parameters.BmerNamespace
// that o have a given label.
func GetPodByLabel(label string) (corev1.Pod, error) {
	podList, err := helper.Apiclient.Pods(parameters.BmerNamespace).List(context.Background(),
		metav1.ListOptions{
			LabelSelector: label})

	if err != nil {
		return corev1.Pod{}, err
	}

	if len(podList.Items) < 1 {
		return corev1.Pod{}, fmt.Errorf("no pod found with label %s under namespace %s", label,
			parameters.BmerNamespace)
	}

	return podList.Items[0], nil
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
	err := helper.Apiclient.List(context.Background(), hwEventList)

	if err != nil {
		return fmt.Errorf("failed to list hw event proxy custom resource definition due to: %w", err)
	}

	for _, result := range hwEventList.Items {
		if result.Name == ranbmerparameters.CustomResourceDefinition {
			return nil
		}
	}

	return fmt.Errorf("failed to find custum resource definition: %v in cluster",
		ranbmerparameters.CustomResourceDefinition)
}

// WaitForEvent waits for specified event to appear in ptp cloud event proxy log.
func WaitForEvent(consumerPod *corev1.Pod, messageID string, startTime time.Time, timeout time.Duration) error {
	interval := 10 * time.Second

	return wait.PollImmediate(interval, timeout, func() (bool, error) {
		logs, err := podUtil.GetLog(helper.Apiclient, consumerPod, time.Since(startTime)+time.Second,
			ranbmerparameters.ConsumerContainerName)
		if err != nil {
			return false, nil
		}

		eventMsgs := getEvents(logs)

		if len(eventMsgs) == 0 {
			log.Println("No event is found.")

			return false, nil
		}

		for _, event := range eventMsgs {
			if strings.Contains(event, messageID) {
				log.Printf("%s is found\n", messageID)

				return true, nil
			}
		}

		log.Printf("Event %s is not found\n", messageID)

		return false, nil
	})
}

// getEvents parses consumer cloud event logs and returns list of events.
func getEvents(eventLog string) []string {
	var eventMsgs []string

	eventLog = strings.ReplaceAll(eventLog, "\\n", "")
	logsSlice := strings.Split(eventLog, "\n")
	r := regexp.MustCompile(`[(received event|published hw event)]\{(.*)\}`)

	for _, line := range logsSlice {
		if r.MatchString(line) && strings.Contains(line, "redfish-event") {
			line = strings.ReplaceAll(line, "\\", "")
			eventMsgs = append(eventMsgs, line)
		}
	}

	return eventMsgs
}
