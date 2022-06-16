package netcniparameters

import multus "gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/types"

var (
	FirstNetworkConfig = multus.NetworkSelectionElement{
		Name:      "test-nad-sysctl-first",
		IPRequest: []string{"10.100.100.200/24"},
	}

	SecondNetworkConfig = multus.NetworkSelectionElement{
		Name:      "test-nad-sysctl-second",
		IPRequest: []string{"10.100.200.200/24"},
	}

	ResourceNameSysctl         = "sriovnicsysctl"
	AllFlagsSysctlPluginConfig = map[string]string{
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
	GlobalSysctlFlag = "kernel.shm_rmid_forced"
)
