package rfclient

import "github.com/stmcginnis/gofish/redfish"

// EventType is Dell's event type for version <1.5.
type (
	// zTsubscriptionPayload to create the event subscription.
	zTsubscriptionPayload struct {
		Destination string                           `json:"Destination"`
		Protocol    redfish.EventDestinationProtocol `json:"Protocol,omitempty"`
		Context     string                           `json:"Context,omitempty"`
	}

	SubscriptionURI string

	// ZtSubscribeResponseType zt uses a unique subscription response.
	ZtSubscribeResponseType struct {
		OdataContext        string `json:"@odata.context"`
		OdataEtag           string `json:"@odata.etag"`
		OdataID             string `json:"@odata.id"`
		OdataType           string `json:"@odata.type"`
		Context             string `json:"Context"`
		DeliveryRetryPolicy string `json:"DeliveryRetryPolicy"`
		Description         string `json:"Description"`
		Destination         string `json:"Destination"`
		EventFormatType     string `json:"EventFormatType"`
		ID                  int    `json:"ID"`
		Name                string `json:"Name"`
		Protocol            string `json:"Protocol"`
		Status              struct {
			Health       string `json:"Health"`
			HealthRollup string `json:"HealthRollup"`
			State        string `json:"State"`
		}
		SubordinateResources bool   `json:"SubordinateResources"`
		SubscriptionType     string `json:"SubscriptionType"`
	}

	// EEMIRegistryType is Dell's events registry.
	// Any of these events can be sent using SubmitEventTest.
	EEMIRegistryType struct {
		OdataContext string `json:"@odata.context"`
		OdataID      string `json:"@odata.id"`
		OdataType    string `json:"@odata.type"`
		Description  string `json:"Description"`
		ID           string `json:"ID"`
		Language     string `json:"Language"`
		Messages     map[string]struct {
			Description     string   `json:"Description"`
			Message         string   `json:"Message"`
			MessageSeverity string   `json:"MessageSeverity"`
			NumberOfArgs    int      `json:"NumberOfArgs"`
			ParamTypes      []string `json:"ParamTypes"`
			Resolution      string   `json:"Resolution"`
			Severity        string   `json:"Severity"`
		}
		MessagesOdataCount int    `json:"Messages@odata.count"`
		Name               string `json:"Name"`
		OwningEntity       string `json:"OwningEntity"`
		RegistryPrefix     string `json:"RegistryPrefix"`
		RegistryVersion    string `json:"RegistryVersion"`
	}

	dellPayloadType struct {
		EventID           string `json:"EventID"`
		EventTimestamp    string `json:"EventTimestamp"`
		EventType         string `json:"EventType"`
		Message           string
		MessageArgs       []string
		MessageID         string `json:"MessageId"`
		OriginOfCondition string
		Severity          string
	}

	hpePayloadType struct {
		EventID           string `json:"EventID"`
		EventTimestamp    string `json:"EventTimestamp"`
		EventType         string `json:"EventType"`
		Message           string
		MessageArgs       []string
		MessageID         string `json:"MessageId"`
		OriginOfCondition string
		Severity          string
	}

	ztPayload struct {
		MessageID string `json:"MessageId"`
	}
)
