package parameters

import k8sv1 "k8s.io/api/core/v1"

var (
	SleepCommand           = []string{"/bin/bash", "-c", "sleep INF"}
	trueVar                = true
	falseVar               = false
	capabilityAll          = []k8sv1.Capability{"ALL"}
	defaultGroupID         = int64(3000)
	defaultUserID          = int64(2000)
	DefaultSecurityContext = k8sv1.SecurityContext{
		AllowPrivilegeEscalation: &falseVar,
		RunAsNonRoot:             &trueVar,
		SeccompProfile:           &k8sv1.SeccompProfile{Type: "RuntimeDefault"},
		Capabilities: &k8sv1.Capabilities{
			Drop: capabilityAll,
		},
		RunAsGroup: &defaultGroupID,
		RunAsUser:  &defaultUserID,
	}
)
