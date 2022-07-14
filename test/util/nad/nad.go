package nad

import (
	"context"
	"encoding/json"
	"fmt"

	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type (
	Capability struct {
		Mac bool `json:"mac,omitempty"`
	}

	Link struct {
		Name string `json:"name,omitempty"`
	}

	IPAM struct {
		Type       string   `json:"type,omitempty"`
		AddrRange  string   `json:"range,omitempty"`
		RangeStart string   `json:"range_start,omitempty"`
		RangeEnd   string   `json:"range_end,omitempty"`
		Gateway    string   `json:"gateway,omitempty"`
		Exclude    []string `json:"exclude,omitempty"`
	}

	Plugin struct {
		LinksInContainer bool              `json:"linksInContainer,omitempty"`
		IPMasq           bool              `json:"ipMasq,omitempty"`
		IsGateway        bool              `json:"isGateway,omitempty"`
		IsDefaultGateway bool              `json:"isDefaultGateway,omitempty"`
		ForceAddress     bool              `json:"forceAddress,omitempty"`
		HairpinMode      bool              `json:"hairpinMode,omitempty"`
		PromiscMode      bool              `json:"promiscMode,omitempty"`
		FailOverMac      int               `json:"failOverMac,omitempty"`
		CNIVersion       string            `json:"cniVersion,omitempty"`
		Name             string            `json:"name,omitempty"`
		Type             string            `json:"type,omitempty"`
		Bridge           string            `json:"bridge,omitempty"`
		Master           string            `json:"master,omitempty"`
		Vlan             string            `json:"vlan,omitempty"`
		Mtu              string            `json:"mtu,omitempty"`
		VrfName          string            `json:"vrfName,omitempty"`
		Mode             string            `json:"mode,omitempty"`
		Miimon           string            `json:"miimon,omitempty"`
		Ipam             *IPAM             `json:"ipam,omitempty"`
		Capabilities     *Capability       `json:"capabilities,omitempty"`
		Sysctl           map[string]string `json:"sysctl,omitempty"`
		Links            []Link            `json:"links,omitempty"`
	}

	MasterPlugin struct {
		CniVersion string    `json:"cniVersion,omitempty"`
		Name       string    `json:"name,omitempty"`
		Type       string    `json:"type,omitempty"`
		Master     string    `json:"master,omitempty"`
		Bridge     string    `json:"bridge,omitempty"`
		Vlan       uint16    `json:"vlanid,omitempty"`
		Mode       string    `json:"mode,omitempty"`
		Ipam       *IPAM     `json:"ipam,omitempty"`
		Plugins    *[]Plugin `json:"plugins,omitempty"`
	}

	NetworkAttachmentDefinitionBuilder struct {
		Definition        nadv1.NetworkAttachmentDefinition
		metaPluginConfigs []Plugin
		ipam              *IPAM
		errorMsg          string
	}
)

// NewNadBuilder creates new instance of NetworkAttachmentDefinitionBuilder.
func NewNadBuilder(name string, namespace string) *NetworkAttachmentDefinitionBuilder {
	return &NetworkAttachmentDefinitionBuilder{
		metaPluginConfigs: []Plugin{},
		Definition: nadv1.NetworkAttachmentDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: nadv1.NetworkAttachmentDefinitionSpec{
				Config: "",
			},
		},
	}
}

// WithIpam adds ipam to NetworkAttachmentDefinition resource.
func (b *NetworkAttachmentDefinitionBuilder) WithIpam(ipam *IPAM) *NetworkAttachmentDefinitionBuilder {
	b.ipam = ipam

	return b
}

// WithSysctTuning adds sysctl tuning plugin to NetworkAttachmentDefinition resource.
func (b *NetworkAttachmentDefinitionBuilder) WithSysctTuning(
	sysctConfig map[string]string) *NetworkAttachmentDefinitionBuilder {
	b.metaPluginConfigs = append(b.metaPluginConfigs, Plugin{
		Type:   "tuning",
		Sysctl: sysctConfig,
	})

	return b
}

// WithPlugin adds plugin to NetworkAttachmentDefinition resource.
func (b *NetworkAttachmentDefinitionBuilder) WithPlugin(plugin *Plugin) *NetworkAttachmentDefinitionBuilder {
	b.metaPluginConfigs = append(b.metaPluginConfigs, *plugin)

	return b
}

// BuildWithMasterPluginString adds master plugin sting to NetworkAttachmentDefinition resource.
func (b *NetworkAttachmentDefinitionBuilder) BuildWithMasterPluginString(
	masterPlugin string) (*NetworkAttachmentDefinitionBuilder, error) {
	if b.errorMsg != "" {
		return nil, fmt.Errorf(b.errorMsg)
	}

	if b.Definition.Spec.Config != "" {
		return nil, fmt.Errorf("error nad spec config is not empty")
	}

	b.Definition.Spec.Config = masterPlugin

	return b, nil
}

// WithPlugins adds list of plugins to NetworkAttachmentDefinition resource.
func (b *NetworkAttachmentDefinitionBuilder) WithPlugins(plugins []*Plugin) *NetworkAttachmentDefinitionBuilder {
	for _, plugin := range plugins {
		b.metaPluginConfigs = append(b.metaPluginConfigs, *plugin)
	}

	return b
}

// Build builds NetworkAttachmentDefinition resource based on given param.
func (b *NetworkAttachmentDefinitionBuilder) Build() (*NetworkAttachmentDefinitionBuilder, error) {
	if b.errorMsg != "" {
		return nil, fmt.Errorf(b.errorMsg)
	}

	nadConfig := MasterPlugin{}
	nadConfig.CniVersion = "0.4.0"
	nadConfig.Name = b.Definition.Name
	nadConfig.Ipam = b.ipam
	nadConfig.Plugins = &b.metaPluginConfigs

	if b.ipam != nil {
		for _, plugin := range *nadConfig.Plugins {
			if plugin.Ipam != nil {
				return nil, fmt.Errorf(
					"invalid config, ipam defined in master plugin as well as in secondary plugin")
			}
		}
	}

	nadConfigJSONString, err := json.Marshal(nadConfig)

	if err != nil {
		return nil, fmt.Errorf("can not marshal cni config")
	}

	b.Definition.Spec.Config = string(nadConfigJSONString)

	return b, nil
}

// GetString prints NetworkAttachmentDefinition resource.
func (b *NetworkAttachmentDefinitionBuilder) GetString() (string, error) {
	nadByte, err := json.MarshalIndent(b.Definition, "", "    ")
	if err != nil {
		return "", err
	}

	return string(nadByte), err
}

// Create creates NetworkAttachmentDefinition resource.
func (b *NetworkAttachmentDefinitionBuilder) Create(clientSet *client.ClientSet) error {
	if b.Definition.Spec.Config == "" {
		return fmt.Errorf("error to create network-attachment-definitions object because it's config is empty")
	}

	if b.errorMsg != "" {
		return fmt.Errorf(b.errorMsg)
	}

	err := clientSet.Create(context.TODO(), &b.Definition)
	if err != nil {
		return fmt.Errorf("fail to create NAD object due to: %w", err)
	}

	return nil
}

// DefineMacVlanPlugin returns mac-vlan plugin config.
func DefineMacVlanPlugin(master string, ipam *IPAM) *Plugin {
	return &Plugin{
		Type:   "macvlan",
		Master: master,
		Ipam:   ipam,
	}
}

func DefineBridgeVlanPlugin(name, master, mode string, vlanID uint16, ipam *IPAM) *MasterPlugin {
	return &MasterPlugin{
		CniVersion: "0.4.0",
		Name:       name,
		Master:     master,
		Mode:       mode,
		Vlan:       vlanID,
		Type:       "vlan",
		Ipam:       ipam,
	}
}

// DefineVrfPlugin returns vrf plugin config.
func DefineVrfPlugin(vrfName string) *Plugin {
	return &Plugin{
		Type:    "vrf",
		VrfName: vrfName,
	}
}

// DefineTuningPluginWithSysctl returns sysctl cni plugin config based on given map.
func DefineTuningPluginWithSysctl(sysctlConfig map[string]string) *Plugin {
	return &Plugin{
		Type:   "tuning",
		Sysctl: sysctlConfig,
	}
}

// DefineStaticIpam returns static ipam config.
func DefineStaticIpam() *IPAM {
	return DefineIpam("static")
}

// DefineIpam returns static ipam config.
func DefineIpam(ipamType string) *IPAM {
	return &IPAM{
		Type: ipamType,
	}
}

// DefineIpamWhereabouts returns whereabouts ipam config.
func DefineIpamWhereabouts(addrRange string) *IPAM {
	return &IPAM{
		Type:      "whereabouts",
		AddrRange: addrRange,
	}
}

// DefineMasterPlugin returns master nad plugin config.
func DefineMasterPlugin(name string, plugin []Plugin) *MasterPlugin {
	return &MasterPlugin{
		Name:       name,
		CniVersion: "0.4.0",
		Plugins:    &plugin,
	}
}

// DefineMasterBridgePlugin returns master nad plugin with bridge config.
func DefineMasterBridgePlugin(name, bridgeName string, ipamConfig *IPAM) *MasterPlugin {
	return &MasterPlugin{
		Name:       name,
		CniVersion: "0.4.0",
		Type:       "bridge",
		Bridge:     bridgeName,
		Ipam:       ipamConfig,
	}
}

// DefineBondPlugin returns master nad plugin config.
func DefineBondPlugin(ipam *IPAM, bondPorts []string, bondMode string) *Plugin {
	bondPlugin := &Plugin{
		Type:             "bond",
		Mode:             bondMode,
		FailOverMac:      1,
		LinksInContainer: true,
		Miimon:           "100",
		Ipam:             ipam,
	}
	for _, bondPort := range bondPorts {
		bondPlugin.Links = append(bondPlugin.Links, Link{Name: bondPort})
	}

	return bondPlugin
}
