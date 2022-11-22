package netcniparameters

import (
	"fmt"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"

	k8sv1 "k8s.io/api/core/v1"
)

var (
	NetworkWithSysctlMutation    = "test-sysct-mutation"
	NetworkWithoutSysctlMutation = "test-no-sysct-mutation"
	FirstNetworkConfig           = *pod.DefinePodNetStaticIP("test-nad-sysctl-first", "10.100.100.200/24")
	SecondNetworkConfig          = *pod.DefinePodNetStaticIP("test-nad-sysctl-second", "10.100.200.200/24")
	ResourceNameSysctl           = "sriovnicsysctl"
	AllFlagsSysctlPluginConfig   = map[string]string{
		"net.ipv4.conf.IFNAME.accept_redirects":        "0",
		"net.ipv4.conf.IFNAME.accept_source_route":     "0",
		"net.ipv4.conf.IFNAME.disable_policy":          "1",
		"net.ipv4.conf.IFNAME.secure_redirects":        "0",
		"net.ipv4.conf.IFNAME.send_redirects":          "0",
		"net.ipv6.conf.IFNAME.accept_redirects":        "0",
		"net.ipv6.conf.IFNAME.accept_source_route":     "1",
		"net.ipv6.neigh.IFNAME.base_reachable_time_ms": "20000",
		"net.ipv6.neigh.IFNAME.retrans_time_ms":        "2000",
	}
	SingleSysctlFlag = map[string]string{
		"net.ipv4.conf.IFNAME.accept_redirects": "0",
	}
	SingleAcceptRedirectSysctlFlag = map[string]string{
		"net.ipv4.conf.IFNAME.accept_redirects": "1",
	}
	InvalidSysctlKey  = "net.ipv4.conf.IFNAME.dfdsfsdf"
	SingleInvalidFlag = map[string]string{
		InvalidSysctlKey: "0",
	}
	MultipleFlagsSyscl = map[string]string{
		"net.ipv4.conf.IFNAME.accept_redirects":    "0",
		"net.ipv4.conf.IFNAME.accept_source_route": "0",
		"net.ipv4.conf.IFNAME.disable_policy":      "1",
		"net.ipv4.conf.IFNAME.secure_redirects":    "0",
	}
	GlobalSysctlFlag           = "kernel.shm_rmid_forced"
	NetAdminSC                 = nethelper.DefineSecurityContext([]k8sv1.Capability{"NET_ADMIN"}, false)
	NetRawSC                   = nethelper.DefineSecurityContext([]k8sv1.Capability{"NET_RAW"}, false)
	ClientNetAdmNetRawSysAdmSC = nethelper.DefineSecurityContext(
		[]k8sv1.Capability{"NET_ADMIN", "NET_RAW", "SYS_ADMIN"}, true)
	BondInterfaceName       = "bond0"
	BondInterfaceNameSecond = "bond1"
	SrvLopIPAddr            = "4.4.4.4"
	SrvInitCMD              = fmt.Sprintf(
		"ip addr add %s/32 dev lo && ip route add blackhole 10.100.100.1/32", SrvLopIPAddr)
	RdrInitCMD     = fmt.Sprintf("ip route add %s/32 via 10.100.100.200", SrvLopIPAddr)
	ClientInitCMDs = fmt.Sprintf(
		"sysctl -w net.ipv4.conf.all.accept_redirects=1 && ip route add %s/32 via 10.100.100.1", SrvLopIPAddr)
	SrvLopSecondIPAddr = "5.5.5.5"
	SrvDualInitCMD     = fmt.Sprintf(
		"%s && ip addr add %s/32 dev lo && ip route add blackhole 10.100.200.1/32", SrvInitCMD, SrvLopSecondIPAddr)
	RdrDualInitCMD    = fmt.Sprintf("%s && ip route add %s/32 via 10.100.200.200", RdrInitCMD, SrvLopSecondIPAddr)
	ClientDualInitCMD = fmt.Sprintf("%s && ip route add %s/32 via 10.100.200.1", ClientInitCMDs, SrvLopSecondIPAddr)
)
