package ranztpparameters

import (
	"time"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/k8sreporter"
)

var (
	// ZtpNamespace contains a list of all the ztp related namespaces.
	// This is currently a placeholder.
	ZtpNamespaces = map[string]string{}
	// ZtpCrds contains a list of all the talm related CRDs.
	// This is currently a placeholder.
	ZtpCrds = []k8sreporter.CRData{}
)

const (
	// Namespaces matching '^ztp*' are special
	// Argocd will only let us use such namespaces for policy gen templates on the SNO nodes.
	ZtpTestNamespace     string        = "ztp-test"
	HubKubeEnvKey        string        = "KUBECONFIG_HUB"
	SpokeKubeEnvKey      string        = "KUBECONFIG"
	ZtpDeployedNamespace string        = "openshift-gitops"
	ZtpDeploymentName    string        = "openshift-gitops-repo-server"
	Policies             string        = "policies"
	MinimumZtpVersion    string        = "4.12"
	ArgocdChangeTimeout  time.Duration = 10 * time.Minute
	ArgocdChangeInterval time.Duration = 10 * time.Second
)
