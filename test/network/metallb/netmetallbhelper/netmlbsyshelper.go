package netmetallbhelper

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/netparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	metallbv1beta1 "go.universe.tf/metallb/api/v1beta1"

	"github.com/nmstate/kubernetes-nmstate/api/shared"
	nmstatev1 "github.com/nmstate/kubernetes-nmstate/api/v1"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmlbparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	"gopkg.in/yaml.v2"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AddRouteToPod adds given static route to the pod.
func AddRouteToPod(apiClient *client.ClientSet, runningPod k8sv1.Pod, dstNetwork, nextHop string) error {
	_, err := pod.ExecCommand(apiClient, runningPod, []string{"ip", "route", "add", dstNetwork, "via", nextHop})
	if err != nil {
		return err
	}

	cmdOutBuf, err := pod.ExecCommand(apiClient, runningPod, []string{"ip", "route", "show", dstNetwork})
	if err != nil {
		return err
	}

	if strings.Contains(cmdOutBuf.String(), dstNetwork) && strings.Contains(cmdOutBuf.String(), nextHop) {
		return nil
	}

	return fmt.Errorf("failed to detect previously installed route")
}

// DefineAndCreateSpeakerBGPPeer defines the BGPPeer config and creates resource on cluster.
func DefineAndCreateSpeakerBGPPeer(peerAddress, bfdProfile string, remoteAsn uint32) (*metallbv1beta1.BGPPeer, error) {
	bgpPeerDefinition := defineSpeakerBGPPeer(peerAddress, remoteAsn, netmlbparameters.EBGPProtocol, bfdProfile)
	bgpPeerDefinition.Spec.ASN = remoteAsn
	err := helper.Apiclient.Create(context.Background(), bgpPeerDefinition)

	return bgpPeerDefinition, err
}

// DefineNMStateRoute sets nmState route configuration based on given arguments.
func DefineNMStateRoute(dstNet, nextHopAddr, nextHopInt string) *netmlbparameters.NMStateRoute {
	return &netmlbparameters.NMStateRoute{
		Destination:      dstNet,
		Metric:           150,
		NextHopAddress:   nextHopAddr,
		NextHopInterface: nextHopInt,
		TableID:          254,
	}
}

// RemoveNmStateConfig sets absent states for interface and route nmState configuration.
func RemoveNmStateConfig(
	existingConf nmstatev1.NodeNetworkConfigurationPolicy) (*nmstatev1.NodeNetworkConfigurationPolicy, error) {
	existingConfigState := netmlbparameters.NMStateConfiguration{}

	err := yaml.Unmarshal([]byte(existingConf.Spec.DesiredState.String()), &existingConfigState)

	if err != nil {
		return nil, err
	}

	for idx := range existingConfigState.Interfaces {
		existingConfigState.Interfaces[idx].State = "absent"
	}

	for idx := range existingConfigState.Routes.Config {
		existingConfigState.Routes.Config[idx].State = "absent"
	}

	existingConfigStateJSON, err := yaml.Marshal(existingConfigState)
	if err != nil {
		return nil, err
	}

	newPolicy := &nmstatev1.NodeNetworkConfigurationPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: existingConf.Name,
		}, Spec: shared.NodeNetworkConfigurationPolicySpec{
			NodeSelector: existingConf.Spec.NodeSelector,
			DesiredState: shared.NewState(string(existingConfigStateJSON)),
		},
	}
	newPolicy.SetResourceVersion(existingConf.GetResourceVersion())

	return newPolicy, nil
}

// DefineAndRunNGINXServer defines NGINX server pod on given worker node.
func DefineAndRunNGINXServer(label, workerName string) *k8sv1.Pod {
	privilegedTrue := true
	tcpDumpContainer := pod.DefineContainer(
		"tcpdump", netmlbparameters.TCPDumpCMD, helper.Config.Network.TestContainerImage,
		&k8sv1.SecurityContext{Privileged: &privilegedTrue})

	return DefineAndRunMlbServerPodWithSecondContainer(workerName,
		helper.Config.Network.TestContainerImage,
		label, []string{netmlbparameters.ArgCommandNGINX}, tcpDumpContainer)
}

// DefineAndCreateBGPAdvertisement configures and creates BGPAdvertisement resource on cluster.
func DefineAndCreateBGPAdvertisement(bgpAdvName, iPAddPoolName, bgpPeerName string) error {
	bgpAdvertisement := DefineBGPAdvertisement(
		bgpAdvName,
		netmlbparameters.CommunityNoAdv,
		netparameters.IPV4Family,
		[]string{iPAddPoolName},
		netmlbparameters.PrefixLen32,
		netmlbparameters.LocalPref100,
	)
	bgpAdvertisement.Spec.Peers = []string{bgpPeerName}

	return helper.Apiclient.Create(context.Background(), bgpAdvertisement)
}

// DefineAndCreateBFDProfile defines and creates bfdProfile.
func DefineAndCreateBFDProfile() (*metallbv1beta1.BFDProfile, error) {
	bfdProfileDefinition := defineBFDProfile(netmlbparameters.BFDProfileName)
	err := helper.Apiclient.Create(context.Background(), bfdProfileDefinition)

	return bfdProfileDefinition, err
}

// DefineNMStatePolicy defines nmState policy based on given nmState interface and nmState route settings.
func DefineNMStatePolicy(policyName, nodeName string,
	interfaces []netmlbparameters.NMStateInterface,
	routes []netmlbparameters.NMStateRoute) (*nmstatev1.NodeNetworkConfigurationPolicy, error) {
	nmStateConfig := defineNMStateConfigWithRoutes(
		*defineNMStateConfigWithInterface(interfaces),
		netmlbparameters.NMStateRoutesConfig{Config: routes})

	nmStateConfigJSON, err := yaml.Marshal(nmStateConfig)

	if err != nil {
		return nil, err
	}

	return &nmstatev1.NodeNetworkConfigurationPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: policyName,
		}, Spec: shared.NodeNetworkConfigurationPolicySpec{
			NodeSelector: map[string]string{parameters.LabelHostname: nodeName},
			DesiredState: shared.NewState(string(nmStateConfigJSON)),
		},
	}, nil
}

// DefineNmStateVlanInterfaceConfig defines nmState configuration for a given interface.
func DefineNmStateVlanInterfaceConfig(
	intFaceName, intFaceAddr string, prefix int, vlanID uint16) *netmlbparameters.NMStateInterface {
	nmStateIPv4Config := defineNMAddress(intFaceAddr, prefix, true, false)
	nmStateVlanA := defineNMVlan(intFaceName, vlanID)

	return DefineNMInterface(intFaceName, "vlan", "up", nmStateIPv4Config, *nmStateVlanA)
}

// GetNodeOvnRouterIP returns router ip address for given worker node.
func GetNodeOvnRouterIP(workerNode *k8sv1.Node) (string, error) {
	routerIPConfig := map[string]map[string]string{}
	err := json.Unmarshal([]byte(workerNode.Annotations["k8s.ovn.org/node-gateway-router-lrp-ifaddrs"]), &routerIPConfig)

	if err != nil {
		return "", err
	}

	routeIP, keyExist := routerIPConfig["default"]["ipv4"]

	if !keyExist {
		return "", fmt.Errorf("annotation %s doesn't have ip configuration", "k8s.ovn.org/node-gateway-router-lrp-ifaddrs")
	}

	routerIPAddress := strings.Split(routeIP, "/")

	if len(routerIPAddress) < 2 {
		return "", fmt.Errorf("can't collect ip address from annotation")
	}

	return routerIPAddress[0], nil
}

// RedefineOnNode mutation function which redefines pods on specific node.
func RedefineOnNode(nodeName string) func(podManifest *k8sv1.Pod) {
	return func(podManifest *k8sv1.Pod) {
		podManifest.Spec.NodeSelector = map[string]string{"kubernetes.io/hostname": nodeName}
	}
}

// CheckNeighborStatus returns information for the given neighbor in the given executor.
func CheckNeighborStatus(frrPod *k8sv1.Pod, neighborsIPAddresses string) bool {
	neighborState, err := pod.ExecCommand(helper.Apiclient, *frrPod,
		append(netmlbparameters.VtyshFRRCmdPrefix, "show ip bgp neighbor json"))
	Expect(err).ToNot(HaveOccurred())

	parseNeigh := parseNeighbors(neighborState.String())

	if len(parseNeigh) < 1 {
		return false
	}

	if parseNeigh[0].IP.Equal(net.ParseIP(neighborsIPAddresses)) && parseNeigh[0].Connected {
		return true
	}

	return false
}

// CheckBGPRoute verifies if given bgp route is present in route table of frr router.
func CheckBGPRoute(frrPod *k8sv1.Pod, neighborsIPAddresses, route, iPFamily string, prefixLen int32) error {
	// bgpStateOut example output after being parsed - map[4.4.4.100:{4.4.4.100/32 [10.46.55.116 10.46.55.115] 100}]
	bgpStateOut, err := pod.ExecCommand(helper.Apiclient, *frrPod, append(netmlbparameters.VtyshFRRCmdPrefix,
		fmt.Sprintf("show bgp %s json", iPFamily)))

	if err != nil {
		return err
	}

	routes, err := parseRoutes(bgpStateOut.String())

	if err != nil {
		return err
	}

	ipRoutes, routePrefix := routes[route]

	if !routePrefix {
		return fmt.Errorf("route %s not found", route)
	}

	if !strings.Contains(ipRoutes.NextHops[0].String(), neighborsIPAddresses) {
		return fmt.Errorf("route %s has invalid nextHop address", route)
	}

	if uint32(prefixLen) != ipRoutes.PrefixLen {
		return fmt.Errorf("advertised prefix %d is not equal to %d", prefixLen, ipRoutes.PrefixLen)
	}

	return err
}

// IcmpConnectivityWorks runs icmp test from source pod to dst IP.
func IcmpConnectivityWorks(apiClient *client.ClientSet, sourcePod k8sv1.Pod, destIP string) error {
	_, err := pod.ExecCommand(apiClient, sourcePod, []string{"ping", destIP, "-c3"})

	return err
}

// SrcAndDestIPInVlanICMPTrafficCapture verify if given arguments are present in icmp traffic capture.
func SrcAndDestIPInVlanICMPTrafficCapture(runningPod *k8sv1.Pod, srcIP, dstIP string, vlan ...uint16) error {
	if len(vlan) > 0 {
		return srcAndDstIPInTrafficCapture(runningPod, srcIP, dstIP, "ICMP", 0, vlan[0])
	}

	return srcAndDstIPInTrafficCapture(runningPod, srcIP, dstIP, "ICMP", 0)
}

// SrcAndDestIPInVlanHTTPTrafficCapture verify if given arguments are preset in vlan http traffic capture.
func SrcAndDestIPInVlanHTTPTrafficCapture(runningPod *k8sv1.Pod, srcIP, dstIP string, vlan uint16) error {
	return srcAndDstIPInTrafficCapture(runningPod, srcIP, dstIP, "http", 0, vlan)
}

// SrcAndDestIPInHTTPTrafficCapture verify if given arguments are present in http traffic capture.
func SrcAndDestIPInHTTPTrafficCapture(runningPod *k8sv1.Pod, srcIP, dstIP string, cIndex ...int) error {
	if len(cIndex) > 0 {
		return srcAndDstIPInTrafficCapture(runningPod, srcIP, dstIP, "http", cIndex[0])
	}

	return srcAndDstIPInTrafficCapture(runningPod, srcIP, dstIP, "http", 0)
}

func srcAndDstIPInTrafficCapture(runningPod *k8sv1.Pod, srcIP, dstIP, proto string, cIndex int, vlan ...uint16) error {
	var trafficPattern = ".80"
	if proto == "ICMP" {
		trafficPattern = ": ICMP"
	}

	trafficCapture, err := pod.ExecCommand(
		helper.Apiclient, *runningPod,
		[]string{"cat", "traffic"}, runningPod.Spec.Containers[cIndex].Name)

	if err != nil {
		return fmt.Errorf("%w: can't open traffic dump file", err)
	}

	for _, line := range strings.Split(trafficCapture.String(), "\n") {
		if strings.Contains(line, srcIP) && strings.Contains(line, fmt.Sprintf("%s%s", dstIP, trafficPattern)) {
			if len(vlan) > 0 {
				if strings.Contains(line, fmt.Sprintf("vlan %d", vlan[0])) {
					return nil
				}

				return fmt.Errorf("failed to detect vlan id in traffic capture")
			}

			return nil
		}
	}

	return fmt.Errorf(
		"failed to detect required src: %s dst: %s ip address in traffic caputure", srcIP, dstIP)
}

func defineNMVlan(ifName string, vlanID uint16) *netmlbparameters.NMStateVlan {
	return &netmlbparameters.NMStateVlan{
		BeseIface: ifName,
		ID:        vlanID,
	}
}

func defineNMStateConfigWithInterface(
	ifaceConfig []netmlbparameters.NMStateInterface) *netmlbparameters.NMStateConfiguration {
	return &netmlbparameters.NMStateConfiguration{
		Interfaces: ifaceConfig,
	}
}

func defineNMStateConfigWithRoutes(
	nmStateConfig netmlbparameters.NMStateConfiguration,
	routeConfig netmlbparameters.NMStateRoutesConfig) *netmlbparameters.NMStateConfiguration {
	nmStateConfig.Routes = routeConfig

	return &nmStateConfig
}

func defineNMAddress(ipv4Address string, prefLength int, enabled, dhcp bool) *netmlbparameters.NMStateIPv4Address {
	return &netmlbparameters.NMStateIPv4Address{
		Address: []map[string]interface{}{
			{
				"ip":            ipv4Address,
				"prefix-length": prefLength,
			},
		},
		Dhcp:    dhcp,
		Enabled: enabled,
	}
}

func DefineNMInterface(
	name, intType, state string, ipv4Config *netmlbparameters.NMStateIPv4Address,
	vlan ...netmlbparameters.NMStateVlan) *netmlbparameters.NMStateInterface {
	nmStateIfaceConfig := &netmlbparameters.NMStateInterface{
		Name:  name,
		Type:  intType,
		State: state,
	}
	if ipv4Config != nil {
		nmStateIfaceConfig.IPv4 = *ipv4Config
	}

	if intType == "vlan" && len(vlan) > 0 {
		nmStateIfaceConfig.Name = fmt.Sprintf("%s.%d", name, vlan[0].ID)
		nmStateIfaceConfig.Vlan = vlan[0]
	}

	return nmStateIfaceConfig
}
