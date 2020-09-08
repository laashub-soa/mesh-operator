package handler

import (
	"time"

	"github.com/symcn/mesh-operator/pkg/adapter/component"
	"github.com/symcn/mesh-operator/pkg/adapter/convert"
	k8sclient "github.com/symcn/mesh-operator/pkg/k8s/client"
	k8smanager "github.com/symcn/mesh-operator/pkg/k8s/manager"
	"github.com/symcn/mesh-operator/pkg/option"
	"github.com/symcn/mesh-operator/pkg/utils"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	ctrlmanager "sigs.k8s.io/controller-runtime/pkg/manager"
)

// Init the handler initialization
func Init(opt option.EventHandlers) ([]component.EventHandler, error) {
	var eventHandlers []component.EventHandler
	// If this flag has been set as true, it means you want to synchronize all services to a kubernetes cluster.
	if opt.EnableK8s {
		cfg := buildRestConfig(opt)
		kubeCli := buildClientSet(cfg)
		ctrlMgr := buildCtrlManager(cfg)

		var (
			eh  component.EventHandler
			err error
		)

		if opt.IsMultiClusters {
			eh, err = buildMultiClusterEventHandler(opt, ctrlMgr, kubeCli)
		} else {
			eh, err = buildSingleClusterEventHandler(opt, ctrlMgr, kubeCli)
		}
		if err != nil {
			klog.Fatalf("Build cluster event handler err:%v", err)
		}

		eventHandlers = append(eventHandlers, eh)
	}

	if opt.EnableDebugLog {
		logHandler, err := NewLogEventHandler()
		if err != nil {
			return nil, err
		}

		eventHandlers = append(eventHandlers, logHandler)
	}

	return eventHandlers, nil
}

func buildRestConfig(opt option.EventHandlers) *rest.Config {
	// deciding which kubeconfig we shall use.
	var cfg *rest.Config
	var err error
	if opt.Kubeconfig == "" {
		cfg, err = config.GetConfig()
	} else {
		cfg, err = k8sclient.GetConfigWithContext(opt.Kubeconfig, opt.ConfigContext)
	}
	if err != nil {
		klog.Fatalf("unable to load the default kubeconfig, err: %v", err)
	}
	return cfg
}

func buildClientSet(cfg *rest.Config) *kubernetes.Clientset {
	// initializing kube client with the config we has decided
	kubeCli, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("failed to get kubernetes Clientset: %v", err)
	}
	return kubeCli
}

func buildCtrlManager(cfg *rest.Config) ctrlmanager.Manager {
	// initializing control manager with the config
	rp := time.Second * 120
	ctrlMgr, err := ctrlmanager.New(cfg, ctrlmanager.Options{
		Scheme:             k8sclient.GetScheme(),
		MetricsBindAddress: "0",
		LeaderElection:     false,
		// Port:               9443,
		SyncPeriod: &rp,
	})
	if err != nil {
		klog.Fatalf("unable to create a manager, err: %v", err)
	}

	klog.Info("starting the control manager")
	stopCh := utils.SetupSignalHandler()
	go func() {
		if err := ctrlMgr.Start(stopCh); err != nil {
			klog.Fatalf("problem start running manager err: %v", err)
		}
	}()
	for !ctrlMgr.GetCache().WaitForCacheSync(stopCh) {
		klog.Warningf("Waiting for caching objects to informer")
		time.Sleep(5 * time.Second)
	}
	klog.Infof("caching objects to informer is successful")
	return ctrlMgr
}

func buildMultiClusterEventHandler(opt option.EventHandlers, ctrlMgr ctrlmanager.Manager, kubeCli *kubernetes.Clientset) (component.EventHandler, error) {
	// it need to synchronize services to the clusters we found with a configmap which is used for
	// defining these clusters
	masterClient := k8smanager.MasterClient{
		KubeCli: kubeCli,
		Manager: ctrlMgr,
	}
	// initializing multiple k8s cluster manager
	klog.Info("start to initializing multiple cluster managers ... ")
	labels := map[string]string{
		"ClusterOwner": opt.ClusterOwner,
	}
	clustersMgrOpt := k8smanager.DefaultClusterManagerOption(opt.ClusterNamespace, labels)
	if opt.ClusterNamespace != "" {
		clustersMgrOpt.Namespace = opt.ClusterNamespace
	}
	k8sMgr, err := k8smanager.NewClusterManager(masterClient, clustersMgrOpt)
	if err != nil {
		klog.Fatalf("unable to create a new k8s manager, err: %v", err)
	}

	// initializing the handlers that you decide to utilize
	kubeMceh, err := NewKubeMultiClusterEventHandler(k8sMgr)
	if err != nil {
		return nil, err
	}
	klog.Info("event handler for synchronizing to multiple clusters has been initialized.")
	return kubeMceh, nil
}

func buildSingleClusterEventHandler(opt option.EventHandlers, ctrlMgr ctrlmanager.Manager, kubeCli *kubernetes.Clientset) (component.EventHandler, error) {
	converter := convert.DubboConverter{DefaultNamespace: defaultNamespace}
	kubeSingleHandler, err := NewKubeSingleClusterEventHandler(ctrlMgr, &converter)
	if err != nil {
		klog.Errorf("Initializing an event handler for synchronizing to multiple clusters has an error: %v", err)
		return nil, err
	}
	klog.Info("event handler for synchronizing to single clusters has been initialized.")
	return kubeSingleHandler, nil
}
