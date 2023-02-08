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

type ArgocdGitDetails struct {
	Repo   string
	Branch string
	Path   string
}

const (
	// Namespaces matching '^ztp*' are special
	// Argocd will only let us use such namespaces for policy gen templates on the SNO nodes.
	ZtpTestNamespace          string        = "ztp-test"
	HubKubeEnvKey             string        = "KUBECONFIG_HUB"
	SpokeKubeEnvKey           string        = "KUBECONFIG"
	OpenshiftGitops           string        = "openshift-gitops"
	OpenshiftGitopsRepoServer string        = "openshift-gitops-repo-server"
	ArgocdPoliciesAppName     string        = "policies"
	ArgocdClustersAppName     string        = "clusters"
	MinimumZtpVersion         string        = "4.12"
	ArgocdChangeTimeout       time.Duration = 10 * time.Minute
	ArgocdChangeInterval      time.Duration = 10 * time.Second
	ZtpSiteGenerateImageName  string        = "registry-proxy.engineering.redhat.com/rh-osbs/openshift4-ztp-site-generate"
	AcmOperatorName           string        = "multiclusterhub-operator"
	AcmOperatorNamespace      string        = "open-cluster-management"
	AcmArgocdInitContainer    string        = "multicluster-operators-subscription"
	AcmPolicyGeneratorName    string        = "acm-policy-generator"
)

// ArgocdApps is a list of the argocd app names that are defined above.
var ArgocdApps = []string{
	ArgocdClustersAppName,
	ArgocdPoliciesAppName,
}
