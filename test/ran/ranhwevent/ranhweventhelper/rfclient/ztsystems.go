package rfclient

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/redfish"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhwevent/ranhweventparameters"
)

// SubscribeZt subscription procedure for zt systems redfish.
func SubscribeZt(client *gofish.APIClient) (SubscriptionURI, error) {
	payload := zTsubscriptionPayload{
		Destination: ranhweventparameters.Redfish.EventReceiver,
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
func SendEventZt(eventService *redfish.EventService, msgID string) error {
	p := ztPayload{
		MessageID: msgID,
	}
	resp, err := eventService.Client.Post(submitTestEventTarget, p)

	if err != nil {
		return fmt.Errorf("failed to send submitTestEvent in SendEventZt() due to: %w", err)
	}
	defer resp.Body.Close()

	valid := map[int]bool{http.StatusAccepted: true}

	if !valid[resp.StatusCode] {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response from send event request due to: %w", err)
		}

		return fmt.Errorf("failed to submit test event due to: %s status code: %v", body, resp.StatusCode)
	}

	return nil
}
