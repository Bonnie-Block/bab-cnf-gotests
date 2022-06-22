package ranhweventhelper

import (
	"encoding/json"
	"log"
	"regexp"
	"strings"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhwevent/ranhweventhelper/rfclient"
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
func getMsgIDType1(eventJSON string) ([]TimestampEventType, error) {
	var (
		events    []TimestampEventType
		eventType rfclient.EventType
	)

	eventUnquote := strings.ReplaceAll(eventJSON, "\\", "")
	err := json.Unmarshal([]byte(eventUnquote), &eventType)

	if err != nil {
		log.Printf("Error when parsing message ID: %s", err)
	} else if len(eventType.Data.Data.Events) > 0 {
		events = append(events, TimestampEventType{
			eventType.Time, eventType.Data.Data.Events[0].MessageID})

	}

	return events, err
}

// IsEventJSON returns a json in case its a valid json or empty string if not.
func IsEventJSON(line string) string {
	r := regexp.MustCompile(`[received event]\{(.*)\}`)

	eventJSON := r.FindString(line)

	if eventJSON == "" {
		return ""
	}

	return eventJSON
}

// getMsgIDType15 parse dell version 1.5 events messages.
func getMsgIDType15(eventJSON string) ([]TimestampEventType, error) {
	var (
		e15    rfclient.EventType15
		events []TimestampEventType
	)

	eventUnquote := strings.ReplaceAll(eventJSON, "\\", "")
	err := json.Unmarshal([]byte(eventUnquote), &e15)

	if err != nil {
		log.Printf("Error when parsing message ID: %s", err)

		return events, err
	}

	for _, value := range e15.Data.Values {
		for _, event := range value.Value.Events {
			events = append(events, TimestampEventType{event.EventTimestamp, event.MessageID})
		}
	}

	return events, nil
}

// GetMsgID returns the message ID from the event that was received.
func GetMsgID(eventJSON string) ([]TimestampEventType, error) {
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

type TimestampEventType struct {
	EventTimestamp, MessageID string
}
