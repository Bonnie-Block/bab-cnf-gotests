package rfclient

import "github.com/stmcginnis/gofish/redfish"

// EventType is Dell's event type for version <1.5.
type (
	EventType struct {
		ID              string `json:"id"`
		Type            string `json:"type"`
		DataContentType string `json:"dataContentType"`
		Time            string `json:"time"`
		Data            struct {
			Version string `json:"version"`
			Data    struct {
				OdataContext string `json:"@odata.context"`
				Context      string `json:"Context"`
				OdataType    string `json:"@odata.type"`
				Events       []struct {
					Context        string   `json:"Context"`
					EventGroupID   int      `json:"EventGroupID"`
					EventID        string   `json:"EventID"`
					EventTimestamp string   `json:"EventTimestamp"`
					Message        string   `json:"Message"`
					MessageArgs    []string `json:"MessageArgs"`
					Severity       string   `json:"Severity"`
					EventType      string   `json:"EventType"`
					MessageID      string   `json:"MessageId"`
					MemberID       string   `json:"MemberID"`
				}
				ID   string `json:"ID"`
				Name string `json:"Name"`
			}
		}
	}

	// EventType15 is Dell's event type for version 1.5.
	EventType15 struct {
		ID              string `json:"id"`
		Type            string `json:"type"`
		Source          string `json:"source"`
		DataContentType string `json:"data_content_type"`
		Time            string `json:"time"`
		Data            struct {
			Version string `json:"version"`
			Values  []struct {
				Resource  string `json:"resource"`
				DataType  string `json:"data_type"`
				ValueType string `json:"value_type"`
				Value     struct {
					OdataContext string `json:"odata_context"`
					Context      string `json:"context"`
					OdataType    string `json:"odata_type"`
					Events       []struct {
						Context        string   `json:"Context"`
						EventGroupID   int      `json:"EventGroupID"`
						EventID        string   `json:"EventID"`
						EventTimestamp string   `json:"EventTimestamp"`
						Message        string   `json:"Message"`
						MessageArgs    []string `json:"MessageArgs"`
						Severity       string   `json:"Severity"`
						EventType      string   `json:"EventType"`
						MessageID      string   `json:"MessageId"`
						MemberID       string   `json:"MemberID"`
					}
					ID   string `json:"id"`
					Name string `json:"name"`
				}
			}
		}
	}

	// zTsubscriptionPayload is the dellPayloadType to create the event subscription.
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

	MsgID string

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
		Destination string                           `json:"Destination"`
		EventTypes  redfish.EventType                `json:"EventTypes"`
		Context     string                           `json:"Context"`
		Protocol    redfish.EventDestinationProtocol `json:"Protocol"`
		MessageID   string                           `json:"MessageId"`
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
