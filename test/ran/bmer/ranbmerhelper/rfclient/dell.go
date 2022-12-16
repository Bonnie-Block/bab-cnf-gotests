package rfclient

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"time"

	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/redfish"
)

// GetIdracEvents query Dell's Idrac registry to get events to use in test.
func GetIdracEvents(c *gofish.APIClient) ([]string, error) {
	var data EEMIRegistryType

	path := "/redfish/v1/Registries/Messages/EEMIRegistry"
	resp, err := c.Get(path)

	if err != nil {
		log.Printf("Failed to get iDrac messeges due to: %s", err)

		return []string{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)

	if err != nil {
		log.Printf("Failed to read Idrac events response due to: %v\n", err)

		return []string{}, err
	}

	err = json.Unmarshal(body, &data)

	if err != nil {
		log.Printf("Failed to decode Idrac events json due to: %v\n", err)

		return []string{}, err
	}

	var ret []string

	for msgID, msg := range data.Messages {
		if msg.Severity != "Informational" {
			ret = append(ret, msgID)
		}
	}

	sort.Strings(ret)
	log.Printf("Found %v event types\n", len(ret))

	return ret, nil
}

// SendEventDell sends event according to msgId and returns error.
func SendEventDell(eventservice *redfish.EventService, msgID string) error {
	payload := dellPayloadType{
		EventID:           "TestEventId",
		EventTimestamp:    time.Now().Format(time.RFC3339), // "2019-07-29T15:13:49Z",
		EventType:         "Alert",                         // redfish.SupportedEventTypes["Alert"],
		Message:           "Test Event",
		MessageID:         msgID,
		OriginOfCondition: "/redfish/v1/Systems/1/",
		Severity:          "OK",
	}
	resp, err := eventservice.Client.Post(submitTestEventTarget, payload)

	if err != nil {
		return fmt.Errorf("failed to send submitTestEvent due to: %w", err)
	}
	defer resp.Body.Close()

	valid := map[int]bool{http.StatusNoContent: true, http.StatusCreated: true}

	if !valid[resp.StatusCode] {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response from send event request due to: %w", err)
		}

		log.Printf("Failed to submit test event due to: %s\n", body)
		log.Printf("Submit test status code: %v\n", resp.StatusCode)
	}

	return nil
}
