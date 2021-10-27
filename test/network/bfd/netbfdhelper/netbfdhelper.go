package netbfdhelper

import (
	"encoding/json"
	"fmt"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/bfd/netbfdparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"

	appsv1 "k8s.io/api/apps/v1"
	k8sv1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type BFDDescription struct {
	BFDStatus string `json:"status"`
	BFDpeer   string `json:"peer"`
}

// IsStatusUp checks if the status of all the BFD peers of a given pod is up.
func IsBFDStatusUp(pod *k8sv1.Pod, peers []string) error {
	if len(peers) == 0 {
		return fmt.Errorf("invalid input, peers size is 0")
	}

	for _, peer := range peers {
		err := IsBFDHasStatus(pod, peer, netbfdparameters.BFDStatusUp)
		if err != nil {
			return err
		}
	}

	return nil
}

// IsBFDHasStatus verifies that BFD session on a pod has given status.
func IsBFDHasStatus(frrPod *k8sv1.Pod, bfdPeer string, status string) error {
	bfdStatusOut, err := pod.ExecCommand(helper.Apiclient, *frrPod,
		[]string{"vtysh", "-c", "sh bfd peers brief json"})
	if err != nil {
		return err
	}

	result := []BFDDescription{}

	err = json.Unmarshal(bfdStatusOut.Bytes(), &result)
	if err != nil {
		return err
	}

	for _, peer := range result {
		if peer.BFDpeer == bfdPeer && peer.BFDStatus != status {
			return fmt.Errorf("%s bfd status is %s (expected %s)", peer.BFDpeer, peer.BFDStatus, status)
		}
	}

	return nil
}

// DefineBFDRoleBinding returns RoleBinding required for the test setup.
func DefineBFDRoleBinding() *rbacv1.RoleBinding {
	roleBinding := &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{
		Name:      netbfdparameters.RoleName,
		Namespace: netbfdparameters.TestNamespace},
		RoleRef: rbacv1.RoleRef{
			Name:     netbfdparameters.RoleName,
			Kind:     "Role",
			APIGroup: "rbac.authorization.k8s.io"},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "default",
				Namespace: netbfdparameters.TestNamespace},
		}}

	return roleBinding
}

// DefineBFDRole returns Role required for the test setup.
func DefineBFDRole() *rbacv1.Role {
	role := &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{
		Name:      netbfdparameters.RoleName,
		Namespace: netbfdparameters.TestNamespace},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups:     []string{"security.openshift.io"},
				ResourceNames: []string{"privileged"},
				Resources:     []string{"securitycontextconstraints"},
				Verbs:         []string{"use"},
			},
		},
	}

	return role
}

// DefineBFDDaemonset returns DaemonSet required for the test setup.
func DefineBFDDaemonset() *appsv1.DaemonSet {
	daemonset := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      netbfdparameters.AppName,
			Namespace: netbfdparameters.TestNamespace,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": netbfdparameters.AppName,
				},
			},
			Template: k8sv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":  netbfdparameters.AppName,
						"name": netbfdparameters.AppName,
					},
				},
				Spec: k8sv1.PodSpec{
					HostNetwork: true,
					Volumes: []k8sv1.Volume{
						{
							Name: netbfdparameters.WorkerConfigMapName,
							VolumeSource: k8sv1.VolumeSource{
								ConfigMap: &k8sv1.ConfigMapVolumeSource{
									LocalObjectReference: k8sv1.LocalObjectReference{
										Name: netbfdparameters.WorkerConfigMapName,
									},
								},
							},
						},
					},
					Containers: []k8sv1.Container{
						{
							Name:  netbfdparameters.AppName,
							Image: helper.Config.Network.FrrImage,
							VolumeMounts: []k8sv1.VolumeMount{
								{
									Name:      netbfdparameters.WorkerConfigMapName,
									MountPath: netbfdparameters.MountPath,
								},
							},
							SecurityContext: &k8sv1.SecurityContext{
								Capabilities: &k8sv1.Capabilities{
									Add: []k8sv1.Capability{
										"NET_RAW",
										"SYS_ADMIN",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	return daemonset
}

// DefineFRRPod returns Pod required for the FRR test setup.
func DefineFRRPod() *k8sv1.Pod {
	pod := &k8sv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      netbfdparameters.PodOnMasterName,
			Namespace: netbfdparameters.TestNamespace,
		},

		Spec: k8sv1.PodSpec{
			HostNetwork: true,
			Volumes: []k8sv1.Volume{
				{
					Name: netbfdparameters.MasterConfigMapName,
					VolumeSource: k8sv1.VolumeSource{
						ConfigMap: &k8sv1.ConfigMapVolumeSource{
							LocalObjectReference: k8sv1.LocalObjectReference{
								Name: netbfdparameters.MasterConfigMapName,
							},
						},
					},
				},
			},
			NodeSelector: map[string]string{
				netbfdparameters.MasterNodeSelector: "",
			},
			Tolerations: []k8sv1.Toleration{
				{
					Key:    netbfdparameters.MasterNodeSelector,
					Effect: "NoSchedule",
				},
			},
			Containers: []k8sv1.Container{
				{
					Name:  netbfdparameters.AppName,
					Image: helper.Config.Network.FrrImage,
					VolumeMounts: []k8sv1.VolumeMount{
						{
							Name:      netbfdparameters.MasterConfigMapName,
							MountPath: netbfdparameters.MountPath,
						},
					},
					SecurityContext: &k8sv1.SecurityContext{
						Capabilities: &k8sv1.Capabilities{
							Add: []k8sv1.Capability{
								"NET_RAW",
								"SYS_ADMIN",
							},
						},
					},
				},
			},
		},
	}

	return pod
}

// defineBFDConfig returns string which represents BFD config file peering to all given IP addresses.
func defineBFDConfig(neighborsIPAddresses *[]string) string {
	bfdConfig := "bfd\n"
	for _, ipAddress := range *neighborsIPAddresses {
		bfdConfig += fmt.Sprintf(netbfdparameters.PeerConfigTemplate, ipAddress)
	}

	bfdConfig += "!"

	return bfdConfig
}

// DefineBFDConfigMap returns configMapName required for the test setup.
func DefineBFDConfigMap(ipAddresses *[]string, configMapName string) *k8sv1.ConfigMap {
	configMapData := make(map[string]string)

	// run the bfd daemon on non-default port, so it won't collide with metallb's bfd.
	configMapData["bfdd.conf"] = "/usr/lib/frr/bfdd --bfdctl /tmp/bfdd.sock\n--dplaneaddr ipv4:127.0.0.1:50701\n"

	configMapData["daemons"] = netbfdparameters.DaemonsFile
	configMapData["vtysh.conf"] = ""
	configMapData["frr.conf"] = defineBFDConfig(ipAddresses)

	configMap := &k8sv1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: netbfdparameters.TestNamespace,
		},
		Data: configMapData,
	}

	return configMap
}
