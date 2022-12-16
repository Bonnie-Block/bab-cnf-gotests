package rfclient

import (
	"fmt"
	"log"

	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/redfish"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/bmer/ranbmerparameters"
)

var (
	eventContext          = "root"
	submitTestEventTarget = "/redfish/v1/EventService/Actions/EventService.SubmitTestEvent"
	ztSubscribeURL        = "/redfish/v1/EventService/Subscriptions"
)

// GetClient returns a redfish session.
func GetClient(config ranbmerparameters.RedfishConfig) (c *gofish.APIClient, err error) {
	if nethelper.IPFamilyForAddress(config.Hostname) == "ipv6" {
		config.RedfishURL = "https://[" + config.Hostname + "]"
	}

	clientConfig := gofish.ClientConfig{
		Endpoint:  config.RedfishURL,
		Username:  config.Username,
		Password:  config.Password,
		Insecure:  true,
		BasicAuth: true,
	}

	return gofish.Connect(clientConfig)
}

// Subscribe the hardware event proxy to receive redfish events.
func Subscribe(
	localNodeVendor string,
	config ranbmerparameters.RedfishConfig) (
	SubscriptionURI,
	*redfish.EventService, error) {
	var (
		uri                 SubscriptionURI
		createdSubscription string
	)

	eventService, err := config.Session.Service.EventService()
	if err != nil {
		return "", nil, fmt.Errorf("failed to Subscribe redfish events due to: %w", err)
	}

	if localNodeVendor == ranbmerparameters.ZT {
		uri, err = SubscribeZt(config.Session)
	} else {
		createdSubscription, err = eventService.CreateEventSubscription(
			ranbmerparameters.Redfish.EventReceiver,
			[]redfish.EventType{redfish.SupportedEventTypes["Alert"]},
			nil,
			redfish.RedfishEventDestinationProtocol,
			eventContext,
			nil, // oem is optional
		)
		uri = SubscriptionURI(createdSubscription)
	}

	if err != nil {
		log.Printf("Failed to subscribe to redfish events due to: %v\n", err)

		return "", nil, err
	}

	log.Printf("Creted Event subscription URI: %v\n", uri)

	return uri, eventService, err
}

// Unsubscribe from receiving the redfish events.
func Unsubscribe(subscriptionURI SubscriptionURI, eventservice *redfish.EventService) error {
	err := eventservice.DeleteEventSubscription(
		string(subscriptionURI))
	if err != nil {
		log.Printf("Calling DeleteEventSubscription failed due to: %v\n", err)

		return err
	}

	log.Printf("Deleted event subscription: %s\n", subscriptionURI)

	return nil
}

// SendEvent ask redfish to simulate a fault and send an event.
func SendEvent(eventservice *redfish.EventService, msgID string, localNodeVendor string) error {
	var err error

	switch localNodeVendor {
	case ranbmerparameters.Dell:
		err = SendEventDell(eventservice, msgID)
	case ranbmerparameters.Hpe:
		err = SendEventHP(eventservice, msgID)
	case ranbmerparameters.ZT:
		err = SendEventZt(eventservice, msgID)
	default:
		err = fmt.Errorf("missing Vendor in SendEvent() for %v", localNodeVendor)
	}

	return err
}

// GetVendorTestEvents get the list of redfish events to be used in the test.
func GetVendorTestEvents(localNodeVendor string,
	config ranbmerparameters.RedfishConfig,
	discoverEvents ...bool) (
	[]string, error) {
	var (
		testEvents []string
		err        error
	)

	switch localNodeVendor {
	case ranbmerparameters.Dell:
		if len(discoverEvents) > 0 {
			testEvents, err = GetIdracEvents(config.Session)
		} else {
			testEvents = ranbmerparameters.IDRACEvents
		}
	case ranbmerparameters.Hpe:
		testEvents = ranbmerparameters.HpEvents
	case ranbmerparameters.ZT:
		testEvents = ranbmerparameters.ZtEvents
	default:
		err = fmt.Errorf("failed to match %v in GetVendorTestEvents()", localNodeVendor)
	}

	return testEvents, err
}

// ClearSubscriptions unsbscribe all existing subscription from a given redfish node.
func ClearSubscriptions(config ranbmerparameters.RedfishConfig) error {
	eventservice, _ := config.Session.Service.EventService()
	subs, err := eventservice.GetEventSubscriptions()

	if err != nil {
		log.Printf("Failed to get redfish event subscriptions due to: %v\n", err)
	}

	for _, s := range subs {
		log.Printf("Existing subscriptions: %v\n", s.Entity.ODataID)
		subURI := SubscriptionURI(s.Entity.ODataID)
		err := Unsubscribe(subURI, eventservice)

		if err != nil {
			return err
		}
	}

	return nil
}

// GetPowerEvents returns the expected events that redfish created for a given vendor
// for power-on and power-off.
func GetPowerEvents(localNodeVendor string) ([]string, []string) {
	var PowerOnEvents, PowerOffEvents []string

	switch localNodeVendor {
	case ranbmerparameters.Dell:
		{
			PowerOnEvents = ranbmerparameters.DellPowerOnEvents
			PowerOffEvents = ranbmerparameters.DellPowerOffEvents
		}
	case ranbmerparameters.Hpe:
		{
			PowerOnEvents = ranbmerparameters.HPEPowerOnEvents
			PowerOffEvents = ranbmerparameters.HPEPowerOffEvents
		}
	default:
		log.Printf("Failed to GetPowerEvents for Vendor %v\n", localNodeVendor)
	}

	return PowerOnEvents, PowerOffEvents
}

// GetSkippedEvents list of events that are periodically happen on the node,
// and are to be skipped in verification.
func GetSkippedEvents(localNodeVendor string) map[string]bool {
	skippedEvents := map[string]bool{}
	if localNodeVendor == ranbmerparameters.ZT {
		skippedEvents = map[string]bool{
			"Task.1.0.Completed": true,
			"Task.1.0.Cancelled": true,
			"Task.1.0.New":       true,
			"Task.1.0.Running":   true,
		}
	} else if localNodeVendor == ranbmerparameters.Dell {
		skippedEvents = map[string]bool{
			"USR0030": true,
		}
	}

	return skippedEvents
}
