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
	DefaultZtpGitRepo string = "http://registry.kni-qe-0.lab.eng.rdu2.redhat.com:3000" +
		"/kni-qe/ztp-site-configs.git"
	DefaultZtpGitBranch  string        = "worker-1-4.12"
	DefaultZtpGitDir     string        = "ztp-site-configs/policygemtemplates/ztp-test"
	ZtpGitRepoEnvKey     string        = "ZTP_GIT_REPO"
	ZtpGitBranchEnvKey   string        = "ZTP_GIT_BRANCH"
	ZtpGitDirEnvKey      string        = "ZTP_GIT_DIR"
	// Namespaces matching '^ztp*' are special
	// Argocd will only let us use such namespaces for policy gen templates on the SNO nodes
	ZtpTestNamespace	 string		   = "ztp-test"
	HubKubeEnvKey        string        = "KUBECONFIG_HUB"
	SpokeKubeEnvKey      string        = "KUBECONFIG"
	OpenshiftGitops      string        = "openshift-gitops"
	Policies             string        = "policies"
	ArgocdChangeTimeout  time.Duration = 10 * time.Minute
	ArgocdChangeInterval time.Duration = 10 * time.Second
)
