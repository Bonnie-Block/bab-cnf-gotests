package netmetallbhelper

import (
	"fmt"
	"strconv"
	"time"

	. "github.com/onsi/gomega"

	netattdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmlbparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/netparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"

	metallbv1beta1 "go.universe.tf/metallb/api/v1beta1"

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
func defineBFDProfile(name string) *metallbv1beta1.BFDProfile {
	return &metallbv1beta1.BFDProfile{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: netmlbparameters.MetalLBOperatorNameSpace,
		},
		Spec: metallbv1beta1.BFDProfileSpec{
			ReceiveInterval:  uint32Ptr(300),
			TransmitInterval: uint32Ptr(300),
			DetectMultiplier: uint32Ptr(3),
			EchoInterval:     uint32Ptr(300),
			EchoMode:         pointer.BoolPtr(false),
			PassiveMode:      pointer.BoolPtr(false),
			MinimumTTL:       uint32Ptr(5),
		},
	}
}

// defineSpeakerBGPPeer defines the bgppeer config.  Speakers are always AS 64500.  For EBGP
// connections the external FRR is changed.
func defineSpeakerBGPPeer(externalAddress string,
	asn uint32, bgpProtocol string,
	bfdProfile string) *metallbv1beta1.BGPPeer {
	var ebgpMultiHop bool

	bgpPeer := &metallbv1beta1.BGPPeer{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "testpeer-",
			Namespace:    netmlbparameters.MetalLBOperatorNameSpace,
		}}

	bgpSpec := &metallbv1beta1.BGPPeerSpec{
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

	_, subnet, _ := nethelper.DefineIPFamily(speakerIP)

	return pod.RedefinePodWithNetwork(pod.RedefineOnMaster(pod.RedefineAsPrivileged(routerPodDefinition)),
		fmt.Sprintf(`[{"name": "%s","ips": ["%s/%s"]},{"name": "%s","ips": ["%s/%s"]}]`,
			externalNetworkName, externalIP, subnet,
			internalNetworkName, internalIP, subnet))
}

// defineFRRPodWithNetworkAndIP returns frr pod definition with network and IP.
func defineFRRPodWithNetworkAndIP(masterNodeName string, ipAddress string, networkName string) *k8sv1.Pod {
	frrPod := DefineFrrPodWithTestContainer(masterNodeName, netmlbparameters.TestNamespace)
	_, subnet, _ := nethelper.DefineIPFamily(ipAddress)

	return pod.RedefinePodWithNetwork(frrPod, fmt.Sprintf(`[{"name": "%s","ips": ["%s/%s"]}]`,
		networkName, ipAddress, subnet))
}

// DefineMacVlanNAD returns macvlan Network Attachment Definition.
func DefineMacVlanNAD(nadName string, masterInterface string) *netattdefv1.NetworkAttachmentDefinition {
	return &netattdefv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nadName,
			Namespace: netmlbparameters.TestNamespace,
		},
		Spec: netattdefv1.NetworkAttachmentDefinitionSpec{
			Config: fmt.Sprintf(`{"cniVersion": "0.3.1",
	"name": "%s",
	"type": "macvlan",
	"master": "%s",
	"mode": "bridge",
	"ipam": {"type": "static"}}`, nadName, masterInterface),
		}}
}

// DefineBridgeNAD returns bridge Network Attachment Definition.
func DefineBridgeNAD() *netattdefv1.NetworkAttachmentDefinition {
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

// DefineMetalLBIPAddressPool defines a MetalLB IPAddressPool using env IP var METALLB_ADDR_LIST
// for the IP address range.
func DefineMetalLBIPAddressPool(
	metalLBIP []string, ipStack string, ipAddressPoolName string) *metallbv1beta1.IPAddressPool {
	IPAddrPool := metallbv1beta1.IPAddressPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ipAddressPoolName,
			Namespace: netmlbparameters.MetalLBOperatorNameSpace,
			Annotations: map[string]string{
				netmlbparameters.MetalLBAddressPool: ipAddressPoolName,
			},
		},
		Spec: metallbv1beta1.IPAddressPoolSpec{
			Addresses: []string{
				fmt.Sprintln(metalLBIP[0], "-", metalLBIP[1]),
			},
		},
	}

	if ipStack == netparameters.DualIPFamily {
		IPAddrPool.Spec.Addresses = append(IPAddrPool.Spec.Addresses,
			fmt.Sprintln(metalLBIP[2], "-", metalLBIP[3]))
	}

	return &IPAddrPool
}

// DefineMetallbAddressPool defines a MetalLB L2 Address Pool using env IP var METALLB_ADDR_LIST
// for the IP address range.
func DefineMetalLBAddressPool(
	metalLBIPAdresses []string, protocol string, addressPoolName string) *metallbv1beta1.AddressPool {
	return &metallbv1beta1.AddressPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      addressPoolName,
			Namespace: netmlbparameters.MetalLBOperatorNameSpace,
			Annotations: map[string]string{
				netmlbparameters.MetalLBAddressPool: addressPoolName,
			},
		},
		Spec: metallbv1beta1.AddressPoolSpec{
			Protocol:  protocol,
			Addresses: metalLBIPAdresses,
		},
	}
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

// createPrivilegedPodMaster creates privileged test pods on master node.
func createPrivilegedPodMaster(image string, masterNodeName string) *k8sv1.Pod {
	podName := fmt.Sprintf("%s-%s", parameters.PrivPodNamespace, masterNodeName)

	volumeType := k8sv1.HostPathUnset
	volSource := k8sv1.VolumeSource{
		HostPath: &k8sv1.HostPathVolumeSource{Path: "/", Type: &volumeType}}

	masterprivilegedPod := pod.RedefineAsPrivileged(pod.DefinePodOnNode(
		parameters.PrivPodNamespace, image, masterNodeName))
	masterprivilegedPod = pod.RedefineWithVolume(
		pod.RedefineWithHostPid(masterprivilegedPod),
		"rootfs", "/rootfs", volSource, false,
	)

	masterprivilegedPod = helper.WaitUntilPodCreatedAndRunning(
		pod.RedefineWithObjectMeta(pod.RedefineOnMaster(masterprivilegedPod), podName, "", nil), 10*time.Minute)

	helper.WaitForPodsHealthy([]*k8sv1.Pod{masterprivilegedPod}, 1*time.Minute)

	return masterprivilegedPod
}

// DefineBGPAdvertisement returns BGPAdvertisement for list of IPAddressPoolnames.
func DefineBGPAdvertisement(name string,
	ipAddressPoolNames []string,
	ipStack string,
	prefixLenght int32, localPref uint32) *metallbv1beta1.BGPAdvertisement {
	bgpAdv := &metallbv1beta1.BGPAdvertisement{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: netmlbparameters.MetalLBOperatorNameSpace,
		},
		Spec: metallbv1beta1.BGPAdvertisementSpec{
			IPAddressPools: ipAddressPoolNames,
			Communities:    []string{netmlbparameters.CommunityNoAdv},
			LocalPref:      localPref,
		},
	}

	switch ipStack {
	case netparameters.IPV4Family:
		if prefixLenght != 0 {
			bgpAdv.Spec.AggregationLength = &prefixLenght
		}
	case netparameters.IPV6Family:
		if prefixLenght != 0 {
			bgpAdv.Spec.AggregationLengthV6 = &prefixLenght
		}
	}

	return bgpAdv
}

// DefineL2Advertisement returns L2Advertisement for list of ipAddressPoolnames.
func DefineL2Advertisement(name string, ipAddressPoolNames []string) *metallbv1beta1.L2Advertisement {
	return &metallbv1beta1.L2Advertisement{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: netmlbparameters.MetalLBOperatorNameSpace,
		},
		Spec: metallbv1beta1.L2AdvertisementSpec{
			IPAddressPools: ipAddressPoolNames,
		},
	}
}

// DefineFrrPodWithTestContainer creates an FRR Pod with a test container.
func DefineFrrPodWithTestContainer(masterNodeName string, namespace string) *k8sv1.Pod {
	frrPod := nethelper.DefineFRRPod(masterNodeName, namespace, false)

	frrPod.Spec.Containers = append(frrPod.Spec.Containers,
		k8sv1.Container{
			Name:  netmlbparameters.TestContainerName,
			Image: helper.Config.Network.TestContainerImage,
			SecurityContext: &k8sv1.SecurityContext{
				Capabilities: &k8sv1.Capabilities{
					Add: []k8sv1.Capability{
						"NET_ADMIN",
						"NET_RAW",
						"SYS_ADMIN",
					},
				},
			},
			Command: parameters.SleepCommand,
		},
	)

	return frrPod
}
