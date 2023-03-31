package rfclient

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/redfish"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/bmer/ranbmerparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranparameters"
	"k8s.io/apimachinery/pkg/util/wait"
)

// SubscribeZt subscription procedure for zt systems redfish.
func SubscribeZt(client *gofish.APIClient) (SubscriptionURI, error) {
	payload := zTsubscriptionPayload{
		Destination: ranbmerparameters.Redfish.EventReceiver,
		Context:     eventContext,
		Protocol:    redfish.RedfishEventDestinationProtocol,
	}
	resp, err := client.Post(ztSubscribeURL, payload)

	if err != nil {
		return "", fmt.Errorf("failed to POST subscribe request to redfish due to %w", err)
	}
	defer resp.Body.Close()

	var subDict ZtSubscribeResponseType

	if resp.StatusCode == http.StatusCreated {
		subDict, err = decodeResponse(resp)
	}

	if err != nil {
		return "", err
	}

	if len(subDict.Name) == 0 {
		log.Printf("Failed to decode the response from the subscription request\n")
	}

	subscriptionURI := SubscriptionURI(fmt.Sprintf("%s/%v", subDict.OdataID, subDict.ID))

	return subscriptionURI, nil
}

func decodeResponse(resp *http.Response) (ZtSubscribeResponseType, error) {
	var ztSubscribeResponse ZtSubscribeResponseType

	body, err := io.ReadAll(resp.Body)

	if err != nil {
		return ZtSubscribeResponseType{}, fmt.Errorf(
			"failed to read response body from subscription request due to: %w", err)
	}

	err = json.Unmarshal(body, &ztSubscribeResponse)

	if err != nil {
		return ZtSubscribeResponseType{}, fmt.Errorf(
			"failed to decode subscription json: %s due to %w", body, err)
	}

	return ztSubscribeResponse, nil
}

// SendEventZt sends event according to msgId and returns error.
// ZT systems redfish current firmware limits the SubmitTestEvent() request rate.
// The first request will be OK, but if another request is sent with-in 5-15 sec, it will get a 400 error response.
// This issue is reported at https://bugzilla.redhat.com/show_bug.cgi?id=2094842
// To work around this limit, we retry sending the request until we get a good response.
func SendEventZt(eventService *redfish.EventService, msgID string) error {
	var (
		err  error
		resp *http.Response
	)

	payload := ztPayload{
		MessageID: msgID,
	}

	err = wait.PollImmediate(ranbmerparameters.ZTSendEventInterval, ranbmerparameters.ZTSendEventTimeout,
		func() (bool, error) {

			// Add a random delay between 1-2 seconds.
			// This get rid of the most of issue from https://bugzilla.redhat.com/show_bug.cgi?id=2094842
			addRandomDelay(1000, 1000)
			resp, err = eventService.Client.Post(submitTestEventTarget, payload)
			if err == nil {
				return true, nil
			} else if ranparameters.DebugTest {
				log.Printf("During SendEventZt() got this error: %v will retry\n", err)
			}

			if resp != nil {
				_ = resp.Body.Close()
			}

			return false, nil
		})
	if err != nil {
		return fmt.Errorf("failed to send event to ZT systems due to: %w", err)
	}
	defer resp.Body.Close()

	valid := map[int]bool{http.StatusAccepted: true}

	if !valid[resp.StatusCode] {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response from send event request due to: %w", err)
		}

		return fmt.Errorf("failed to submit test event due to: %s status code: %v", body, resp.StatusCode)
	}

	return nil
}

func addRandomDelay(baseMilsec int, rangeMilsec int) {
	rand.Seed(time.Now().UnixNano())
	r := rand.Intn(rangeMilsec)
	time.Sleep(time.Duration(r+baseMilsec) * time.Millisecond)
}
