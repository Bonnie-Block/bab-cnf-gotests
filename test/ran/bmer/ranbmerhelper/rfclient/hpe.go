package rfclient

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/stmcginnis/gofish/redfish"
)

// SendEventHP sends event according to msgId and returns error
// more info https://hewlettpackard.github.io/iLOAmpPack-Redfish-API-Docs/#submitting-a-test-event
func SendEventHP(eventservice *redfish.EventService, msgID string) error {
	payload := hpePayloadType{
		EventID:           "TestEventId",
		EventTimestamp:    time.Now().Format(time.RFC3339), // "2019-07-29T15:13:49Z",
		EventType:         "Alert",                         // redfish.SupportedEventTypes["Alert"],
		Message:           "Test Event",
		MessageArgs:       []string{"NoAMS", "Busy", "Cached"},
		MessageID:         msgID,
		OriginOfCondition: "/redfish/v1/Systems/1/",
		Severity:          "OK",
	}
	resp, err := eventservice.Client.Post(submitTestEventTarget, payload)

	if err != nil {
		log.Printf("Failed to send submitTestEvent due to: %v\n", err)

		return err
	}
	defer resp.Body.Close()

	valid := map[int]bool{http.StatusOK: true, http.StatusNoContent: true, http.StatusCreated: true}

	if !valid[resp.StatusCode] {
		return fmt.Errorf("failed to submit test event due to: %s status code: %v", resp.Body, resp.StatusCode)
	}

	return nil
}
