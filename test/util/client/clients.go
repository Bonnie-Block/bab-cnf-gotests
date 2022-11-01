package client

import (
	"os"

	"github.com/golang/glog"

	multinetpolicyapiv1 "github.com/k8snetworkplumbingwg/multi-networkpolicy/pkg/apis/k8s.cni.cncf.io/v1beta1"
	multinetpolicyclientv1 "github.com/k8snetworkplumbingwg/multi-networkpolicy/pkg/client/clientset/versioned/typed/k8s.cni.cncf.io/v1beta1"
	netattdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	clientnetattdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/client/clientset/versioned/typed/k8s.cni.cncf.io/v1"
	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	clientsriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/pkg/client/clientset/versioned/typed/sriovnetwork/v1"
	whereaboutsScheme "github.com/k8snetworkplumbingwg/whereabouts/pkg/client/clientset/versioned/scheme"
	whereaboutsApi "github.com/k8snetworkplumbingwg/whereabouts/pkg/client/clientset/versioned/typed/whereabouts.cni.cncf.io/v1alpha1"

	metallboperatorv1beta1 "github.com/metallb/metallb-operator/api/v1beta1"
	cguv1alpha1 "github.com/openshift-kni/cluster-group-upgrades-operator/pkg/generated/clientset/versioned/typed/clustergroupupgradesoperator/v1alpha1"
	performancev2 "github.com/openshift-kni/performance-addon-operators/api/v2"
	operv1 "github.com/openshift/api/operator/v1"
	clientconfigv1 "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	routev1 "github.com/openshift/client-go/route/clientset/versioned/typed/route/v1"
	mcv1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	clientmachineconfigv1 "github.com/openshift/machine-config-operator/pkg/generated/clientset/versioned/typed/machineconfiguration.openshift.io/v1"
	ptpv1 "github.com/openshift/ptp-operator/pkg/client/clientset/versioned/typed/ptp/v1"
	olm2 "github.com/operator-framework/operator-lifecycle-manager/pkg/api/client/clientset/versioned/scheme"
	olm "github.com/operator-framework/operator-lifecycle-manager/pkg/api/client/clientset/versioned/typed/operators/v1alpha1"
	bmerv1alpha1 "github.com/redhat-cne/hw-event-proxy-operator/api/v1alpha1"
	fecv2 "github.com/smart-edge-open/sriov-fec-operator/sriov-fec/api/v2"
	metallbv1beta1 "go.universe.tf/metallb/api/v1beta1"
	apiext "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	discovery "k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes/scheme"
	appsv1client "k8s.io/client-go/kubernetes/typed/apps/v1"
	batchv1client "k8s.io/client-go/kubernetes/typed/batch/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	networkv1client "k8s.io/client-go/kubernetes/typed/networking/v1"
	rbacv1client "k8s.io/client-go/kubernetes/typed/rbac/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	policiesv1beta1 "open-cluster-management.io/governance-policy-propagator/api/v1beta1"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ClientSet provides the struct to talk with relevant API.
type ClientSet struct {
	batchv1client.BatchV1Interface
	corev1client.CoreV1Interface
	clientconfigv1.ConfigV1Interface
	clientmachineconfigv1.MachineconfigurationV1Interface
	networkv1client.NetworkingV1Client
	cguv1alpha1.ClustergroupupgradesoperatorV1alpha1Interface
	multinetpolicyclientv1.K8sCniCncfIoV1beta1Interface
	appsv1client.AppsV1Interface
	discovery.DiscoveryInterface
	rbacv1client.RbacV1Interface
	clientsriovv1.SriovnetworkV1Interface
	Config *rest.Config
	runtimeclient.Client
	ptpv1.PtpV1Interface
	olm.OperatorsV1alpha1Interface
	clientnetattdefv1.K8sCniCncfIoV1Interface
	routev1.RouteV1Interface
	whereaboutsApi.WhereaboutsV1alpha1Interface
}

// New returns a *ClientBuilder with the given kubeconfig.
func New(kubeconfig string) *ClientSet {
	var (
		config *rest.Config
		err    error
	)

	if kubeconfig == "" {
		kubeconfig = os.Getenv("KUBECONFIG")
	}

	if kubeconfig != "" {
		glog.V(4).Infof("Loading kube client config from path %q", kubeconfig)
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	} else {
		glog.V(4).Infof("Using in-cluster kube client config")
		config, err = rest.InClusterConfig()
	}

	if err != nil {
		return nil
	}

	clientSet := &ClientSet{}
	clientSet.BatchV1Interface = batchv1client.NewForConfigOrDie(config)
	clientSet.CoreV1Interface = corev1client.NewForConfigOrDie(config)
	clientSet.ConfigV1Interface = clientconfigv1.NewForConfigOrDie(config)
	clientSet.MachineconfigurationV1Interface = clientmachineconfigv1.NewForConfigOrDie(config)
	clientSet.AppsV1Interface = appsv1client.NewForConfigOrDie(config)
	clientSet.DiscoveryInterface = discovery.NewDiscoveryClientForConfigOrDie(config)
	clientSet.SriovnetworkV1Interface = clientsriovv1.NewForConfigOrDie(config)
	clientSet.NetworkingV1Client = *networkv1client.NewForConfigOrDie(config)
	clientSet.PtpV1Interface = ptpv1.NewForConfigOrDie(config)
	clientSet.RbacV1Interface = rbacv1client.NewForConfigOrDie(config)
	clientSet.OperatorsV1alpha1Interface = olm.NewForConfigOrDie(config)
	clientSet.K8sCniCncfIoV1Interface = clientnetattdefv1.NewForConfigOrDie(config)
	clientSet.RouteV1Interface = routev1.NewForConfigOrDie(config)
	clientSet.ClustergroupupgradesoperatorV1alpha1Interface = cguv1alpha1.NewForConfigOrDie(config)
	clientSet.K8sCniCncfIoV1beta1Interface = multinetpolicyclientv1.NewForConfigOrDie(config)
	clientSet.WhereaboutsV1alpha1Interface = whereaboutsApi.NewForConfigOrDie(config)

	clientSet.Config = config

	crScheme := runtime.NewScheme()
	if err := scheme.AddToScheme(crScheme); err != nil {
		panic(err)
	}

	if err := netattdefv1.SchemeBuilder.AddToScheme(crScheme); err != nil {
		panic(err)
	}

	if err := scheme.AddToScheme(crScheme); err != nil {
		panic(err)
	}

	if err := sriovv1.AddToScheme(crScheme); err != nil {
		panic(err)
	}

	if err := mcv1.AddToScheme(crScheme); err != nil {
		panic(err)
	}

	if err := fecv2.AddToScheme(crScheme); err != nil {
		panic(err)
	}

	if err := apiext.AddToScheme(crScheme); err != nil {
		panic(err)
	}

	if err := metallboperatorv1beta1.AddToScheme(crScheme); err != nil {
		panic(err)
	}

	if err := metallbv1beta1.AddToScheme(crScheme); err != nil {
		panic(err)
	}

	if err := performancev2.AddToScheme(crScheme); err != nil {
		panic(err)
	}

	if err := olm2.AddToScheme(crScheme); err != nil {
		panic(err)
	}

	if err := operv1.AddToScheme(crScheme); err != nil {
		panic(err)
	}

	if err := bmerv1alpha1.AddToScheme(crScheme); err != nil {
		panic(err)
	}

	if err := policiesv1.AddToScheme(crScheme); err != nil {
		panic(err)
	}

	if err := multinetpolicyapiv1.AddToScheme(crScheme); err != nil {
		panic(err)
	}

	if err := placementrulev1.AddToScheme(crScheme); err != nil {
		panic(err)
	}

	if err := whereaboutsScheme.AddToScheme(crScheme); err != nil {
		panic(err)
	}

	if err := policiesv1beta1.AddToScheme(crScheme); err != nil {
		panic(err)
	}

	clientSet.Client, err = runtimeclient.New(config, runtimeclient.Options{
		Scheme: crScheme,
	})
	if err != nil {
		panic(err)
	}

	return clientSet
}
