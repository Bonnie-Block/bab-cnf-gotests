package netmetallbhelper

import (
	"fmt"
	"strconv"
	"strings"

	. "github.com/onsi/gomega"

	netattdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	"github.com/metallb/metallb-operator/api/v1beta1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmlbparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/netparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"

	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

// defineBFDMLBConfigMap returns configmap definition with FRR BFD configuration.
func defineBFDMLBConfigMap(ipAddresses []string, configMapName string, asn int, bgpProtocol string) *k8sv1.ConfigMap {
	configMapData := make(map[string]string)
	configMapData["daemons"] = netmlbparameters.DaemonsFile

	Expect(len(ipAddresses)).ToNot(Equal(0))
	bfdConfig := defineBFDConfig(ipAddresses, asn, bgpProtocol)

	configMapData["frr.conf"] = bfdConfig
	configMap := nethelper.DefineFRRBFDConfigMap(configMapName, netmlbparameters.TestNamespace, configMapData)

	return configMap
}

// defineBFDProfile returns BFDprofile definition.
func defineBFDProfile(name string) *v1beta1.BFDProfile {
	return &v1beta1.BFDProfile{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: netmlbparameters.MetalLBOperatorNameSpace,
		},
		Spec: v1beta1.BFDProfileSpec{
			ReceiveInterval:  uint32Ptr(100),
			TransmitInterval: uint32Ptr(100),
			DetectMultiplier: uint32Ptr(3),
			EchoInterval:     uint32Ptr(100),
			EchoMode:         pointer.BoolPtr(true),
			PassiveMode:      pointer.BoolPtr(false),
			MinimumTTL:       uint32Ptr(5),
		},
	}
}

// defineSpeakerBGPPeer defines the bgppeer config.  Speakers are always AS 64500.  For EBGP
// connections the external FRR is changed.
func defineSpeakerBGPPeer(externalAddress string, asn uint32, bgpProtocol string, bfdProfile string) *v1beta1.BGPPeer {
	var ebgpMultiHop bool

	bgpPeer := &v1beta1.BGPPeer{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "testpeer-",
			Namespace:    netmlbparameters.MetalLBOperatorNameSpace,
		}}

	bgpSpec := &v1beta1.BGPPeerSpec{
		Port:     179,
		Password: netmlbparameters.BGPPassword,
		Address:  externalAddress,
	}

	if bfdProfile != "" {
		switch bgpProtocol {
		case netmlbparameters.IBPGPProtocol:
			asn = netmlbparameters.IBGPASN
		case netmlbparameters.EBGPProtocol:
			ebgpMultiHop = true
			asn = netmlbparameters.EBGPASN
		}

		bgpSpec.MyASN = netmlbparameters.IBGPASN
		bgpSpec.ASN = asn
		bgpSpec.EBGPMultiHop = ebgpMultiHop
		bgpSpec.BFDProfile = bfdProfile
	}

	if bfdProfile == "" {
		bgpSpec.MyASN = asn
		bgpSpec.ASN = netmlbparameters.IBGPASN
	}

	bgpPeer.Spec = *bgpSpec

	return bgpPeer
}

// DefineRouterPod returns router pod definition for multihop scenario.
func DefineRouterPod(nodeName string,
	serviceIP string,
	speakerIP string,
	externalNetworkName string,
	internalNetworkName string,
	externalIP string,
	internalIP string) *k8sv1.Pod {
	routerPodDefinition := pod.RedefineWithCommand(pod.DefinePodOnNode(netmlbparameters.TestNamespace,
		helper.Config.Network.FrrImage, nodeName),
		[]string{"/bin/bash", "-c"},
		[]string{fmt.Sprintf("ip route add %s via %s && sleep INF", serviceIP, speakerIP)})

	subnet := netparameters.IPV4Subnet
	if strings.Contains(speakerIP, ":") {
		subnet = netparameters.IPV6Subnet
	}

	return pod.RedefinePodWithNetwork(pod.RedefineOnMaster(pod.RedefineAsPrivileged(routerPodDefinition)),
		fmt.Sprintf(`[{"name": "%s","ips": ["%s/%s"]},{"name": "%s","ips": ["%s/%s"]}]`,
			externalNetworkName, externalIP, subnet,
			internalNetworkName, internalIP, subnet))
}

// defineFRRPodWithNetworkAndIP returns frr pod definition with network and IP.
func defineFRRPodWithNetworkAndIP(masterNodeName string, ipAddress string, networkName string) *k8sv1.Pod {
	frrPod := nethelper.DefineFRRPod(masterNodeName, netmlbparameters.TestNamespace, false)

	return pod.RedefinePodWithNetwork(frrPod, fmt.Sprintf(`[{"name": "%s","ips": ["%s/%s"]}]`,
		networkName, ipAddress, netparameters.IPV4Subnet))
}

// DefineExternalNAD returns external Network Attachment Definition for multihop scenario.
func DefineExternalNAD() *netattdefv1.NetworkAttachmentDefinition {
	return &netattdefv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      netmlbparameters.ExternalNADName,
			Namespace: netmlbparameters.TestNamespace,
		},
		Spec: netattdefv1.NetworkAttachmentDefinitionSpec{
			Config: `{"cniVersion": "0.3.1",
	"name": "externalnad",
	"type": "macvlan",
	"master": "br-ex",
	"mode": "bridge",
	"ipam": {"type": "static"}}`,
		}}
}

// DefineInternalNAD returns external Network Attachment Definition for multihop scenario.
func DefineInternalNAD() *netattdefv1.NetworkAttachmentDefinition {
	return &netattdefv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      netmlbparameters.InternalNADName,
			Namespace: netmlbparameters.TestNamespace,
		},
		Spec: netattdefv1.NetworkAttachmentDefinitionSpec{
			Config: `{"cniVersion": "0.3.1", "name": "internalnad", "type": "bridge", "bridge": "br0",
"ipam": {"type": "static"}}`,
		}}
}

// DefineMetallbAddressPool defines a MetalLB L2 Address Pool using env IP var METALLB_ADDR_LIST
// for the IP address range.
func DefineMetalLBAddressPool(
	metalLBIP []string, protocol string, iPStack string, addressPoolName string) *v1beta1.AddressPool {
	addrPool := v1beta1.AddressPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      addressPoolName,
			Namespace: netmlbparameters.MetalLBOperatorNameSpace,
			Annotations: map[string]string{
				netmlbparameters.MetalLBAddressPool: addressPoolName,
			},
		},
		Spec: v1beta1.AddressPoolSpec{
			Protocol: protocol,
			Addresses: []string{
				fmt.Sprintln(metalLBIP[0], "-", metalLBIP[1]),
			},
		},
	}

	if iPStack == netmlbparameters.DualIPStack {
		addrPool.Spec.Addresses = append(addrPool.Spec.Addresses,
			fmt.Sprintln(metalLBIP[2], "-", metalLBIP[3]))
	}

	return &addrPool
}

// defineBFDConfig returns string which represents BFD config file peering to all given IP addresses.
func defineBFDConfig(neighborsIPAddresses []string, asn int, bgpProtocol string) string {
	asnStr := strconv.Itoa(asn)
	bfdConfig := `!
frr defaults traditional
hostname frr-pod
log file /tmp/frr.log
log timestamp precision 3
!
debug zebra nht
debug bgp neighbor-events
!
bfd
!
`

	switch bgpProtocol {
	case netmlbparameters.EBGPProtocol:
		bfdConfig += fmt.Sprintf("router bgp %d\n", netmlbparameters.EBGPASN)
	case netmlbparameters.IBPGPProtocol:
		bfdConfig += fmt.Sprintf("router bgp %d\n", netmlbparameters.IBGPASN)
	}

	bfdConfig += ` bgp router-id 10.10.10.11
  no bgp ebgp-requires-policy
  no bgp default ipv4-unicast
  no bgp network import-check
`

	for _, ipAddress := range neighborsIPAddresses {
		bfdConfig += fmt.Sprintf("  neighbor %s remote-as %s\n  neighbor %s bfd\n  neighbor %s password %s\n",
			ipAddress, asnStr, ipAddress, ipAddress, netmlbparameters.BGPPassword)
	}

	if bgpProtocol == netmlbparameters.EBGPProtocol {
		for _, ipAddress := range neighborsIPAddresses {
			bfdConfig += fmt.Sprintf("  neighbor %s ebgp-multihop 2\n", ipAddress)
		}
	}

	bfdConfig += "!\naddress-family ipv4 unicast\n"
	for _, ipAddress := range neighborsIPAddresses {
		bfdConfig += fmt.Sprintf("  neighbor %s activate\n", ipAddress)
	}

	bfdConfig += "exit-address-family\n!\naddress-family ipv6 unicast\n"

	for _, ipAddress := range neighborsIPAddresses {
		bfdConfig += fmt.Sprintf("  neighbor %s activate\n", ipAddress)
	}

	bfdConfig += "exit-address-family\n!\nline vty\n!\nend\n"

	return bfdConfig
}
