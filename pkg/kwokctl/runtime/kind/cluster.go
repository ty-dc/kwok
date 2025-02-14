/*
Copyright 2022 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package kind

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"

	"sigs.k8s.io/kwok/pkg/apis/internalversion"
	"sigs.k8s.io/kwok/pkg/consts"
	"sigs.k8s.io/kwok/pkg/kwokctl/components"
	"sigs.k8s.io/kwok/pkg/kwokctl/dryrun"
	"sigs.k8s.io/kwok/pkg/kwokctl/k8s"
	"sigs.k8s.io/kwok/pkg/kwokctl/runtime"
	"sigs.k8s.io/kwok/pkg/log"
	"sigs.k8s.io/kwok/pkg/utils/exec"
	"sigs.k8s.io/kwok/pkg/utils/file"
	"sigs.k8s.io/kwok/pkg/utils/format"
	"sigs.k8s.io/kwok/pkg/utils/net"
	"sigs.k8s.io/kwok/pkg/utils/path"
	"sigs.k8s.io/kwok/pkg/utils/slices"
	"sigs.k8s.io/kwok/pkg/utils/version"
	"sigs.k8s.io/kwok/pkg/utils/wait"
	"sigs.k8s.io/kwok/pkg/utils/yaml"
)

// Cluster is an implementation of Runtime for kind
type Cluster struct {
	*runtime.Cluster

	runtime string
}

// NewDockerCluster creates a new Runtime for kind with docker
func NewDockerCluster(name, workdir string) (runtime.Runtime, error) {
	return &Cluster{
		Cluster: runtime.NewCluster(name, workdir),
		runtime: consts.RuntimeTypeDocker,
	}, nil
}

// NewPodmanCluster creates a new Runtime for kind with podman
func NewPodmanCluster(name, workdir string) (runtime.Runtime, error) {
	return &Cluster{
		Cluster: runtime.NewCluster(name, workdir),
		runtime: consts.RuntimeTypePodman,
	}, nil
}

// Available  checks whether the runtime is available.
func (c *Cluster) Available(ctx context.Context) error {
	return c.Exec(ctx, c.runtime, "version")
}

func (c *Cluster) setup(ctx context.Context, env *env) error {
	conf := &env.kwokctlConfig.Options

	pkiPath := c.GetWorkdirPath(runtime.PkiName)
	if !file.Exists(pkiPath) {
		sans := []string{}
		ips, err := net.GetAllIPs()
		if err != nil {
			logger := log.FromContext(ctx)
			logger.Warn("failed to get all ips", "err", err)
		} else {
			sans = append(sans, ips...)
		}
		if len(conf.KubeApiserverCertSANs) != 0 {
			sans = append(sans, conf.KubeApiserverCertSANs...)
		}
		err = c.MkdirAll(pkiPath)
		if err != nil {
			return fmt.Errorf("failed to create pki dir: %w", err)
		}
		err = c.GeneratePki(pkiPath, sans...)
		if err != nil {
			return fmt.Errorf("failed to generate pki: %w", err)
		}
	}

	pkiEtcd := filepath.Join(pkiPath, "etcd")
	err := c.MkdirAll(pkiEtcd)
	if err != nil {
		return fmt.Errorf("failed to create pki dir: %w", err)
	}
	return nil
}

// https://github.com/kubernetes-sigs/kind/blob/7b017b2ce14a7fdea9d3ed2fa259c38c927e2dd1/pkg/internal/runtime/runtime.go
func (c *Cluster) withProviderEnv(ctx context.Context) context.Context {
	provider := c.runtime
	ctx = exec.WithEnv(ctx, []string{
		"KIND_EXPERIMENTAL_PROVIDER=" + provider,
	})
	return ctx
}

type env struct {
	kwokctlConfig        *internalversion.KwokctlConfiguration
	verbosity            log.Level
	schedulerConfigPath  string
	auditLogPath         string
	auditPolicyPath      string
	prometheusConfigPath string

	inClusterOnHostKubeconfigPath string
	workdir                       string
	caCertPath                    string
	adminKeyPath                  string
	adminCertPath                 string

	kwokConfigPath string
}

func (c *Cluster) env(ctx context.Context) (*env, error) {
	config, err := c.Config(ctx)
	if err != nil {
		return nil, err
	}

	inClusterOnHostKubeconfigPath := "/etc/kubernetes/admin.conf"
	schedulerConfigPath := "/etc/kubernetes/scheduler.conf"
	prometheusConfigPath := "/etc/prometheus/prometheus.yaml"
	kwokConfigPath := "/etc/kwok/kwok.yaml"
	auditLogPath := ""
	auditPolicyPath := ""
	if config.Options.KubeAuditPolicy != "" {
		auditLogPath = c.GetLogPath(runtime.AuditLogName)
		auditPolicyPath = c.GetWorkdirPath(runtime.AuditPolicyName)
	}

	logger := log.FromContext(ctx)
	verbosity := logger.Level()

	pkiPath := "/etc/kubernetes/pki"

	workdir := c.Workdir()
	caCertPath := path.Join(pkiPath, "ca.crt")
	adminKeyPath := path.Join(pkiPath, "admin.key")
	adminCertPath := path.Join(pkiPath, "admin.crt")

	return &env{
		kwokctlConfig:                 config,
		verbosity:                     verbosity,
		schedulerConfigPath:           schedulerConfigPath,
		prometheusConfigPath:          prometheusConfigPath,
		auditLogPath:                  auditLogPath,
		auditPolicyPath:               auditPolicyPath,
		inClusterOnHostKubeconfigPath: inClusterOnHostKubeconfigPath,
		workdir:                       workdir,
		caCertPath:                    caCertPath,
		adminKeyPath:                  adminKeyPath,
		adminCertPath:                 adminCertPath,
		kwokConfigPath:                kwokConfigPath,
	}, nil
}

// Install installs the cluster
func (c *Cluster) Install(ctx context.Context) error {
	err := c.Cluster.Install(ctx)
	if err != nil {
		return err
	}

	env, err := c.env(ctx)
	if err != nil {
		return err
	}

	// This is not necessary when creating a cluster use kind, but in Linux the cluster is created as root,
	// and the files here may not have permissions when deleted, so we create them first.
	err = c.setup(ctx, env)
	if err != nil {
		return err
	}

	err = c.addKind(ctx, env)
	if err != nil {
		return err
	}

	err = c.addEtcd(ctx, env)
	if err != nil {
		return err
	}

	err = c.addKubeApiserver(ctx, env)
	if err != nil {
		return err
	}

	err = c.addKubeControllerManager(ctx, env)
	if err != nil {
		return err
	}

	err = c.addKubeScheduler(ctx, env)
	if err != nil {
		return err
	}

	err = c.addKwokController(ctx, env)
	if err != nil {
		return err
	}

	err = c.addDashboard(ctx, env)
	if err != nil {
		return err
	}

	err = c.addPrometheus(ctx, env)
	if err != nil {
		return err
	}

	err = c.addJaeger(ctx, env)
	if err != nil {
		return err
	}

	err = c.setupPrometheusConfig(ctx, env)
	if err != nil {
		return err
	}

	images, err := c.listAllImages(ctx)
	if err != nil {
		return err
	}

	err = c.pullAllImages(ctx, env, images)
	if err != nil {
		return err
	}

	return nil
}

func (c *Cluster) addKind(ctx context.Context, env *env) (err error) {
	logger := log.FromContext(ctx)
	conf := &env.kwokctlConfig.Options
	var featureGates []string
	var runtimeConfig []string
	if conf.KubeFeatureGates != "" {
		featureGates = strings.Split(strings.ReplaceAll(conf.KubeFeatureGates, "=", ": "), ",")
	}
	if conf.KubeRuntimeConfig != "" {
		runtimeConfig = strings.Split(strings.ReplaceAll(conf.KubeRuntimeConfig, "=", ": "), ",")
	}

	pkiPath := c.GetWorkdirPath(runtime.PkiName)
	err = c.MkdirAll(pkiPath)
	if err != nil {
		return err
	}

	manifestsPath := c.GetWorkdirPath(runtime.ManifestsName)
	err = c.MkdirAll(manifestsPath)
	if err != nil {
		return err
	}

	if conf.KubeAuditPolicy != "" {
		err = c.MkdirAll(c.GetWorkdirPath("logs"))
		if err != nil {
			return err
		}

		err = c.CreateFile(env.auditLogPath)
		if err != nil {
			return err
		}

		err = c.CopyFile(conf.KubeAuditPolicy, env.auditPolicyPath)
		if err != nil {
			return err
		}
	}

	schedulerConfigPath := ""
	if !conf.DisableKubeScheduler && conf.KubeSchedulerConfig != "" {
		schedulerConfigPath = c.GetWorkdirPath(runtime.SchedulerConfigName)
		err = c.CopySchedulerConfig(conf.KubeSchedulerConfig, schedulerConfigPath, env.schedulerConfigPath)
		if err != nil {
			return err
		}
	}

	kubeApiserverTracingConfigPath := ""
	if conf.JaegerPort != 0 {
		kubeApiserverTracingConfigData, err := k8s.BuildKubeApiserverTracingConfig(k8s.BuildKubeApiserverTracingConfigParam{
			Endpoint: conf.BindAddress + ":4317",
		})
		if err != nil {
			return fmt.Errorf("failed to generate kubeApiserverTracingConfig yaml: %w", err)
		}
		kubeApiserverTracingConfigPath = c.GetWorkdirPath(runtime.ApiserverTracingConfig)

		err = c.WriteFile(kubeApiserverTracingConfigPath, []byte(kubeApiserverTracingConfigData))
		if err != nil {
			return fmt.Errorf("failed to write kubeApiserverTracingConfig yaml: %w", err)
		}
	}

	var prometheusPatches internalversion.ComponentPatches
	if conf.PrometheusPort != 0 {
		prometheusPatches = runtime.GetComponentPatches(env.kwokctlConfig, consts.ComponentPrometheus)
		prometheusConfigPath := c.GetWorkdirPath(runtime.Prometheus)

		prometheusPatches.ExtraVolumes = append(prometheusPatches.ExtraVolumes, internalversion.Volume{
			Name:      "prometheus-config",
			HostPath:  prometheusConfigPath,
			MountPath: env.prometheusConfigPath,
		})
	}

	kubeVersion, err := version.ParseVersion(conf.KubeVersion)
	if err != nil {
		return err
	}

	etcdComponentPatches := runtime.GetComponentPatches(env.kwokctlConfig, consts.ComponentEtcd)
	kubeApiserverComponentPatches := runtime.GetComponentPatches(env.kwokctlConfig, consts.ComponentKubeApiserver)
	kubeSchedulerComponentPatches := runtime.GetComponentPatches(env.kwokctlConfig, consts.ComponentKubeScheduler)
	kubeControllerManagerComponentPatches := runtime.GetComponentPatches(env.kwokctlConfig, consts.ComponentKubeControllerManager)
	kwokControllerComponentPatches := runtime.GetComponentPatches(env.kwokctlConfig, consts.ComponentKwokController)
	extraLogVolumes := runtime.GetLogVolumes(ctx)
	kwokControllerExtraVolumes := kwokControllerComponentPatches.ExtraVolumes
	kwokControllerExtraVolumes = append(kwokControllerExtraVolumes, extraLogVolumes...)
	if len(etcdComponentPatches.ExtraEnvs) > 0 ||
		len(kubeApiserverComponentPatches.ExtraEnvs) > 0 ||
		len(kubeSchedulerComponentPatches.ExtraEnvs) > 0 ||
		len(kubeControllerManagerComponentPatches.ExtraEnvs) > 0 {
		logger.Warn("extraEnvs config in etcd, kube-apiserver, kube-scheduler or kube-controller-manager is not supported in kind")
	}
	kindYaml, err := BuildKind(BuildKindConfig{
		BindAddress:                   conf.BindAddress,
		KubeApiserverPort:             conf.KubeApiserverPort,
		EtcdPort:                      conf.EtcdPort,
		JaegerPort:                    conf.JaegerPort,
		DashboardPort:                 conf.DashboardPort,
		PrometheusPort:                conf.PrometheusPort,
		KwokControllerPort:            conf.KwokControllerPort,
		FeatureGates:                  featureGates,
		RuntimeConfig:                 runtimeConfig,
		AuditPolicy:                   env.auditPolicyPath,
		AuditLog:                      env.auditLogPath,
		SchedulerConfig:               schedulerConfigPath,
		TracingConfigPath:             kubeApiserverTracingConfigPath,
		Workdir:                       c.Workdir(),
		Verbosity:                     env.verbosity,
		EtcdExtraArgs:                 etcdComponentPatches.ExtraArgs,
		EtcdExtraVolumes:              etcdComponentPatches.ExtraVolumes,
		ApiserverExtraArgs:            kubeApiserverComponentPatches.ExtraArgs,
		ApiserverExtraVolumes:         kubeApiserverComponentPatches.ExtraVolumes,
		SchedulerExtraArgs:            kubeSchedulerComponentPatches.ExtraArgs,
		SchedulerExtraVolumes:         kubeSchedulerComponentPatches.ExtraVolumes,
		ControllerManagerExtraArgs:    kubeControllerManagerComponentPatches.ExtraArgs,
		ControllerManagerExtraVolumes: kubeControllerManagerComponentPatches.ExtraVolumes,
		KwokControllerExtraVolumes:    kwokControllerExtraVolumes,
		PrometheusExtraVolumes:        prometheusPatches.ExtraVolumes,
		DisableQPSLimits:              conf.DisableQPSLimits,
		KubeVersion:                   kubeVersion,
	})
	if err != nil {
		return err
	}
	err = c.WriteFile(c.GetWorkdirPath(runtime.KindName), []byte(kindYaml))
	if err != nil {
		return fmt.Errorf("failed to write %s: %w", runtime.KindName, err)
	}

	return nil
}

func (c *Cluster) addEtcd(_ context.Context, env *env) (err error) {
	env.kwokctlConfig.Components = append(env.kwokctlConfig.Components, internalversion.Component{
		Name: consts.ComponentEtcd,
		Metric: &internalversion.ComponentMetric{
			Scheme:             "https",
			Host:               "127.0.0.1:2379",
			Path:               "/metrics",
			CertPath:           "/etc/kubernetes/pki/apiserver-etcd-client.crt",
			KeyPath:            "/etc/kubernetes/pki/apiserver-etcd-client.key",
			InsecureSkipVerify: true,
		},
	})
	return nil
}

func (c *Cluster) addKubeApiserver(_ context.Context, env *env) (err error) {
	env.kwokctlConfig.Components = append(env.kwokctlConfig.Components, internalversion.Component{
		Name: consts.ComponentKubeApiserver,
		Metric: &internalversion.ComponentMetric{
			Scheme:             "https",
			Host:               "127.0.0.1:6443",
			Path:               "/metrics",
			CertPath:           "/etc/kubernetes/pki/admin.crt",
			KeyPath:            "/etc/kubernetes/pki/admin.key",
			InsecureSkipVerify: true,
		},
	})
	return nil
}

func (c *Cluster) addKubeControllerManager(_ context.Context, env *env) (err error) {
	conf := &env.kwokctlConfig.Options
	if !conf.DisableKubeControllerManager {
		env.kwokctlConfig.Components = append(env.kwokctlConfig.Components, internalversion.Component{
			Name: consts.ComponentKubeControllerManager,
			Metric: &internalversion.ComponentMetric{
				Scheme:             "https",
				Host:               "127.0.0.1:10257",
				Path:               "/metrics",
				CertPath:           "/etc/kubernetes/pki/admin.crt",
				KeyPath:            "/etc/kubernetes/pki/admin.key",
				InsecureSkipVerify: true,
			},
		})
	}
	return nil
}

func (c *Cluster) addKubeScheduler(_ context.Context, env *env) (err error) {
	conf := &env.kwokctlConfig.Options
	if !conf.DisableKubeScheduler {
		env.kwokctlConfig.Components = append(env.kwokctlConfig.Components, internalversion.Component{
			Name: consts.ComponentKubeScheduler,
			Metric: &internalversion.ComponentMetric{
				Scheme:             "https",
				Host:               "127.0.0.1:10259",
				Path:               "/metrics",
				CertPath:           "/etc/kubernetes/pki/admin.crt",
				KeyPath:            "/etc/kubernetes/pki/admin.key",
				InsecureSkipVerify: true,
			},
		})
	}
	return nil
}

func (c *Cluster) addKwokController(ctx context.Context, env *env) (err error) {
	conf := &env.kwokctlConfig.Options

	// Configure the kwok-controller
	kwokControllerVersion, err := c.ParseVersionFromImage(ctx, c.runtime, conf.KwokControllerImage, "kwok")
	if err != nil {
		return err
	}

	kwokControllerComponentPatches := runtime.GetComponentPatches(env.kwokctlConfig, consts.ComponentKwokController)
	kwokControllerComponentPatches.ExtraVolumes, err = runtime.ExpandVolumesHostPaths(kwokControllerComponentPatches.ExtraVolumes)
	if err != nil {
		return fmt.Errorf("failed to expand host volumes for kwok controller component: %w", err)
	}

	logVolumes := runtime.GetLogVolumes(ctx)
	logVolumes = slices.Map(logVolumes, func(v internalversion.Volume) internalversion.Volume {
		v.HostPath = path.Join("/var/components/controller", v.HostPath)
		return v
	})

	kwokControllerExtraVolumes := kwokControllerComponentPatches.ExtraVolumes
	kwokControllerExtraVolumes = append(kwokControllerExtraVolumes, logVolumes...)

	kwokControllerComponent := components.BuildKwokControllerComponent(components.BuildKwokControllerComponentConfig{
		Runtime:                           conf.Runtime,
		ProjectName:                       c.Name(),
		Workdir:                           env.workdir,
		Image:                             conf.KwokControllerImage,
		Version:                           kwokControllerVersion,
		BindAddress:                       net.PublicAddress,
		Port:                              conf.KwokControllerPort,
		ConfigPath:                        env.kwokConfigPath,
		KubeconfigPath:                    env.inClusterOnHostKubeconfigPath,
		CaCertPath:                        env.caCertPath,
		AdminCertPath:                     env.adminCertPath,
		AdminKeyPath:                      env.adminKeyPath,
		NodeIP:                            "$(POD_IP)",
		NodeName:                          "kwok-controller.kube-system.svc",
		ManageNodesWithAnnotationSelector: "kwok.x-k8s.io/node=fake",
		Verbosity:                         env.verbosity,
		NodeLeaseDurationSeconds:          40,
		EnableCRDs:                        conf.EnableCRDs,
		EnableStageForRefs:                conf.EnableStageForRefs,
		ExtraArgs:                         kwokControllerComponentPatches.ExtraArgs,
		ExtraVolumes:                      kwokControllerExtraVolumes,
		ExtraEnvs:                         kwokControllerComponentPatches.ExtraEnvs,
	})

	pod := components.ConvertToPod(kwokControllerComponent)
	pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env, corev1.EnvVar{
		Name: "POD_IP",
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: "status.podIP",
			},
		},
	})
	kwokControllerPod, err := yaml.Marshal(pod)
	if err != nil {
		return fmt.Errorf("failed to marshal kwok controller pod: %w", err)
	}
	err = c.WriteFile(path.Join(c.GetWorkdirPath(runtime.ManifestsName), consts.ComponentKwokController+".yaml"), kwokControllerPod)
	if err != nil {
		return fmt.Errorf("failed to write: %w", err)
	}

	env.kwokctlConfig.Components = append(env.kwokctlConfig.Components, kwokControllerComponent)
	return nil
}

func (c *Cluster) addDashboard(ctx context.Context, env *env) (err error) {
	conf := &env.kwokctlConfig.Options

	if conf.DashboardPort != 0 {
		dashboardVersion, err := c.ParseVersionFromImage(ctx, c.runtime, conf.DashboardImage, "")
		if err != nil {
			return err
		}

		dashboardComponent, err := components.BuildDashboardComponent(components.BuildDashboardComponentConfig{
			Runtime:        conf.Runtime,
			Workdir:        env.workdir,
			Image:          conf.DashboardImage,
			Version:        dashboardVersion,
			BindAddress:    net.PublicAddress,
			KubeconfigPath: env.inClusterOnHostKubeconfigPath,
			CaCertPath:     env.caCertPath,
			AdminCertPath:  env.adminCertPath,
			AdminKeyPath:   env.adminKeyPath,
			Port:           8000,
			Banner:         fmt.Sprintf("Welcome to %s", c.Name()),
		})
		if err != nil {
			return fmt.Errorf("failed to build dashboard component: %w", err)
		}

		dashboardPod, err := yaml.Marshal(components.ConvertToPod(dashboardComponent))
		if err != nil {
			return fmt.Errorf("failed to marshal dashboard pod: %w", err)
		}
		err = c.WriteFile(path.Join(c.GetWorkdirPath(runtime.ManifestsName), consts.ComponentDashboard+".yaml"), dashboardPod)
		if err != nil {
			return fmt.Errorf("failed to write: %w", err)
		}
		env.kwokctlConfig.Components = append(env.kwokctlConfig.Components, dashboardComponent)
	}
	return nil
}

func (c *Cluster) setupPrometheusConfig(_ context.Context, env *env) (err error) {
	conf := &env.kwokctlConfig.Options

	// Configure the prometheus
	if conf.PrometheusPort != 0 {
		prometheusData, err := components.BuildPrometheus(components.BuildPrometheusConfig{
			Components: env.kwokctlConfig.Components,
		})
		if err != nil {
			return fmt.Errorf("failed to generate prometheus yaml: %w", err)
		}
		prometheusConfigPath := c.GetWorkdirPath(runtime.Prometheus)
		err = c.WriteFile(prometheusConfigPath, []byte(prometheusData))
		if err != nil {
			return fmt.Errorf("failed to write prometheus yaml: %w", err)
		}
	}
	return nil
}

func (c *Cluster) addPrometheus(ctx context.Context, env *env) (err error) {
	conf := &env.kwokctlConfig.Options

	if conf.PrometheusPort != 0 {
		prometheusVersion, err := c.ParseVersionFromImage(ctx, c.runtime, conf.PrometheusImage, "")
		if err != nil {
			return err
		}

		prometheusComponentPatches := runtime.GetComponentPatches(env.kwokctlConfig, consts.ComponentPrometheus)
		prometheusComponentPatches.ExtraVolumes, err = runtime.ExpandVolumesHostPaths(prometheusComponentPatches.ExtraVolumes)
		if err != nil {
			return fmt.Errorf("failed to expand host volumes for prometheus component: %w", err)
		}

		prometheusComponentPatches.ExtraVolumes = append(prometheusComponentPatches.ExtraVolumes,
			internalversion.Volume{
				HostPath:  "/etc/kubernetes/pki/apiserver-etcd-client.crt",
				MountPath: "/etc/kubernetes/pki/apiserver-etcd-client.crt",
				ReadOnly:  true,
			},
			internalversion.Volume{
				HostPath:  "/etc/kubernetes/pki/apiserver-etcd-client.key",
				MountPath: "/etc/kubernetes/pki/apiserver-etcd-client.key",
				ReadOnly:  true,
			},
		)

		prometheusComponent, err := components.BuildPrometheusComponent(components.BuildPrometheusComponentConfig{
			Runtime:       conf.Runtime,
			Workdir:       env.workdir,
			Image:         conf.PrometheusImage,
			Version:       prometheusVersion,
			BindAddress:   net.PublicAddress,
			Port:          9090,
			ConfigPath:    "/var/components/prometheus/etc/prometheus/prometheus.yaml",
			AdminCertPath: env.adminCertPath,
			AdminKeyPath:  env.adminKeyPath,
			Verbosity:     env.verbosity,
			ExtraArgs:     prometheusComponentPatches.ExtraArgs,
			ExtraVolumes:  prometheusComponentPatches.ExtraVolumes,
			ExtraEnvs:     prometheusComponentPatches.ExtraEnvs,
		})
		if err != nil {
			return err
		}

		prometheusPod, err := yaml.Marshal(components.ConvertToPod(prometheusComponent))
		if err != nil {
			return fmt.Errorf("failed to marshal prometheus pod: %w", err)
		}
		err = c.WriteFile(path.Join(c.GetWorkdirPath(runtime.ManifestsName), consts.ComponentPrometheus+".yaml"), prometheusPod)
		if err != nil {
			return fmt.Errorf("failed to write: %w", err)
		}

		env.kwokctlConfig.Components = append(env.kwokctlConfig.Components, prometheusComponent)
	}
	return nil
}

func (c *Cluster) addJaeger(ctx context.Context, env *env) error {
	conf := &env.kwokctlConfig.Options

	if conf.JaegerPort != 0 {
		jaegerVersion, err := c.ParseVersionFromImage(ctx, c.runtime, conf.JaegerImage, "")
		if err != nil {
			return err
		}

		jaegerComponentPatches := runtime.GetComponentPatches(env.kwokctlConfig, consts.ComponentJaeger)
		jaegerComponentPatches.ExtraVolumes, err = runtime.ExpandVolumesHostPaths(jaegerComponentPatches.ExtraVolumes)
		if err != nil {
			return fmt.Errorf("failed to expand host volumes for jaeger component: %w", err)
		}
		jaegerComponent, err := components.BuildJaegerComponent(components.BuildJaegerComponentConfig{
			Runtime:      conf.Runtime,
			Workdir:      env.workdir,
			Image:        conf.JaegerImage,
			Version:      jaegerVersion,
			BindAddress:  net.PublicAddress,
			Port:         16686,
			Verbosity:    env.verbosity,
			ExtraArgs:    jaegerComponentPatches.ExtraArgs,
			ExtraVolumes: jaegerComponentPatches.ExtraVolumes,
		})
		if err != nil {
			return err
		}

		jaegerPod, err := yaml.Marshal(components.ConvertToPod(jaegerComponent))
		if err != nil {
			return fmt.Errorf("failed to marshal jaeger pod: %w", err)
		}
		err = c.WriteFile(path.Join(c.GetWorkdirPath(runtime.ManifestsName), consts.ComponentJaeger+".yaml"), jaegerPod)
		if err != nil {
			return fmt.Errorf("failed to write: %w", err)
		}

		env.kwokctlConfig.Components = append(env.kwokctlConfig.Components, jaegerComponent)
	}
	return nil
}

// Up starts the cluster.
func (c *Cluster) Up(ctx context.Context) error {
	config, err := c.Config(ctx)
	if err != nil {
		return err
	}
	conf := &config.Options

	logger := log.FromContext(ctx)

	if conf.DisableKubeScheduler {
		defer func() {
			err := c.StopComponent(ctx, consts.ComponentKubeScheduler)
			if err != nil {
				logger.Error("Failed to disable kube-scheduler", err)
			}
		}()
	}

	if conf.DisableKubeControllerManager {
		defer func() {
			err := c.StopComponent(ctx, consts.ComponentKubeScheduler)
			if err != nil {
				logger.Error("Failed to disable kube-scheduler", err)
			}
		}()
	}

	err = c.SetConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to set config: %w", err)
	}

	// This needs to be done before starting the cluster
	err = c.Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	kindPath, err := c.preDownloadKind(ctx)
	if err != nil {
		return err
	}

	images, err := c.listAllImages(ctx)
	if err != nil {
		return err
	}

	args := []string{
		"create", "cluster",
		"--config", c.GetWorkdirPath(runtime.KindName),
		"--name", c.Name(),
		"--image", conf.KindNodeImage,
	}

	deadline, ok := ctx.Deadline()
	if ok {
		wait := time.Until(deadline)
		if wait < 0 {
			wait = time.Minute
		}
		args = append(args, "--wait", format.HumanDuration(wait))
	} else {
		args = append(args, "--wait", "1m")
	}

	err = c.Exec(exec.WithAllWriteToErrOut(c.withProviderEnv(ctx)), kindPath, args...)
	if err != nil {
		return err
	}

	err = c.loadImages(ctx, kindPath, images, conf.CacheDir)
	if err != nil {
		return err
	}

	// TODO: remove this when kind support set server
	err = c.fillKubeconfigContextServer(conf.BindAddress)
	if err != nil {
		return err
	}

	kubeconfigPath := c.GetWorkdirPath(runtime.InHostKubeconfigName)

	kubeconfigBuf := bytes.NewBuffer(nil)
	err = c.Kubectl(exec.WithWriteTo(ctx, kubeconfigBuf), "config", "view", "--minify=true", "--raw=true")
	if err != nil {
		return err
	}

	err = c.WriteFile(kubeconfigPath, kubeconfigBuf.Bytes())
	if err != nil {
		return err
	}

	// Cordoning the node to prevent fake pods from being scheduled on it
	err = c.Kubectl(ctx, "cordon", c.getClusterName())
	if err != nil {
		logger.Error("Failed cordon node", err)
	}

	return nil
}

func (c *Cluster) pullAllImages(ctx context.Context, env *env, images []string) error {
	conf := &env.kwokctlConfig.Options
	images = append([]string{
		conf.KindNodeImage,
	}, images...)

	err := c.PullImages(ctx, c.runtime, images, conf.QuietPull)
	if err != nil {
		return err
	}
	return nil
}

func (c *Cluster) listAllImages(ctx context.Context) ([]string, error) {
	config, err := c.Config(ctx)
	if err != nil {
		return nil, err
	}
	conf := &config.Options
	images := []string{conf.KwokControllerImage}
	if conf.DashboardPort != 0 {
		images = append(images, conf.DashboardImage)
	}
	if conf.PrometheusPort != 0 {
		images = append(images, conf.PrometheusImage)
	}
	if conf.JaegerPort != 0 {
		images = append(images, conf.JaegerImage)
	}

	return images, nil
}

// loadDockerImages loads docker images into the cluster.
// `kind load docker-image`
func (c *Cluster) loadDockerImages(ctx context.Context, command string, kindCluster string, images []string) error {
	logger := log.FromContext(ctx)
	for _, image := range images {
		err := c.Exec(c.withProviderEnv(ctx),
			command, "load", "docker-image",
			image,
			"--name", kindCluster,
		)
		if err != nil {
			return err
		}
		logger.Info("Loaded image", "image", image)
	}
	return nil
}

// loadArchiveImages loads docker images into the cluster.
// `kind load image-archive`
func (c *Cluster) loadArchiveImages(ctx context.Context, command string, kindCluster string, images []string, runtime string, tmpDir string) error {
	logger := log.FromContext(ctx)
	for _, image := range images {
		archive := path.Join(tmpDir, "image-archive", strings.ReplaceAll(image, ":", "/")+".tar")
		err := c.MkdirAll(filepath.Dir(archive))
		if err != nil {
			return err
		}

		err = c.Exec(ctx, runtime, "save", image, "-o", archive)
		if err != nil {
			return err
		}
		err = c.Exec(c.withProviderEnv(ctx),
			command, "load", "image-archive",
			archive,
			"--name", kindCluster,
		)
		if err != nil {
			return err
		}
		logger.Info("Loaded image", "image", image)
		err = c.Remove(archive)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Cluster) loadImages(ctx context.Context, kindPath string, images []string, cacheDir string) error {
	var err error
	if c.runtime == consts.RuntimeTypeDocker {
		err = c.loadDockerImages(ctx, kindPath, c.Name(), images)
	} else {
		err = c.loadArchiveImages(ctx, kindPath, c.Name(), images, c.runtime, cacheDir)
	}
	if err != nil {
		return err
	}
	return nil
}

// WaitReady waits for the cluster to be ready.
func (c *Cluster) WaitReady(ctx context.Context, timeout time.Duration) error {
	if c.IsDryRun() {
		return nil
	}

	var (
		err     error
		waitErr error
		ready   bool
	)
	logger := log.FromContext(ctx)
	waitErr = wait.Poll(ctx, func(ctx context.Context) (bool, error) {
		ready, err = c.Ready(ctx)
		if err != nil {
			logger.Debug("Cluster is not ready",
				"err", err,
			)
		}
		return ready, nil
	},
		wait.WithTimeout(timeout),
		wait.WithContinueOnError(10),
		wait.WithInterval(time.Second/2),
	)
	if err != nil {
		return err
	}
	if waitErr != nil {
		return waitErr
	}
	return nil
}

// Ready returns true if the cluster is ready
func (c *Cluster) Ready(ctx context.Context) (bool, error) {
	ok, err := c.Cluster.Ready(ctx)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}

	out := bytes.NewBuffer(nil)
	err = c.KubectlInCluster(exec.WithWriteTo(ctx, out), "get", "pod", "--namespace=kube-system", "--field-selector=status.phase!=Running", "--output=json")
	if err != nil {
		return false, err
	}

	var data corev1.PodList
	err = json.Unmarshal(out.Bytes(), &data)
	if err != nil {
		return false, err
	}

	if len(data.Items) != 0 {
		logger := log.FromContext(ctx)
		logger.Debug("Components not all running",
			"components", slices.Map(data.Items, func(item corev1.Pod) interface{} {
				return struct {
					Pod   string
					Phase string
				}{
					Pod:   log.KObj(&item).String(),
					Phase: string(item.Status.Phase),
				}
			}),
		)
		return false, nil
	}
	return true, nil
}

// Down stops the cluster
func (c *Cluster) Down(ctx context.Context) error {
	kindPath, err := c.preDownloadKind(ctx)
	if err != nil {
		return err
	}

	logger := log.FromContext(ctx)
	err = c.Exec(exec.WithAllWriteToErrOut(c.withProviderEnv(ctx)), kindPath, "delete", "cluster", "--name", c.Name())
	if err != nil {
		logger.Error("Failed to delete cluster", err)
	}

	return nil
}

// Start starts the cluster
func (c *Cluster) Start(ctx context.Context) error {
	err := c.Exec(ctx, c.runtime, "start", c.getClusterName())
	if err != nil {
		return err
	}
	return nil
}

// Stop stops the cluster
func (c *Cluster) Stop(ctx context.Context) error {
	err := c.Exec(ctx, c.runtime, "stop", c.getClusterName())
	if err != nil {
		return err
	}
	return nil
}

var startImportantComponents = map[string]struct{}{
	consts.ComponentEtcd: {},
}

var stopImportantComponents = map[string]struct{}{
	consts.ComponentEtcd:          {},
	consts.ComponentKubeApiserver: {},
}

// StartComponent starts a component in the cluster
func (c *Cluster) StartComponent(ctx context.Context, name string) error {
	logger := log.FromContext(ctx)
	logger = logger.With("component", name)
	if _, important := startImportantComponents[name]; !important {
		if !c.IsDryRun() {
			if _, exist, err := c.inspectComponent(ctx, name); err != nil {
				return err
			} else if exist {
				logger.Debug("Component already started")
				return nil
			}
		}
	}

	logger.Debug("Starting component")
	err := c.Exec(ctx, c.runtime, "exec", c.getClusterName(), "mv", "/etc/kubernetes/"+name+".yaml.bak", "/etc/kubernetes/manifests/"+name+".yaml")
	if err != nil {
		return err
	}
	if _, important := startImportantComponents[name]; important {
		return nil
	}
	if c.IsDryRun() {
		return nil
	}
	return c.waitComponentReady(ctx, name, true, 120*time.Second)
}

// StopComponent stops a component in the cluster
func (c *Cluster) StopComponent(ctx context.Context, name string) error {
	logger := log.FromContext(ctx)
	logger = logger.With("component", name)
	if _, important := stopImportantComponents[name]; !important {
		if !c.IsDryRun() {
			if _, exist, err := c.inspectComponent(ctx, name); err != nil {
				return err
			} else if !exist {
				logger.Debug("Component already stopped")
				return nil
			}
		}
	}

	logger.Debug("Stopping component")
	err := c.Exec(ctx, c.runtime, "exec", c.getClusterName(), "mv", "/etc/kubernetes/manifests/"+name+".yaml", "/etc/kubernetes/"+name+".yaml.bak")
	if err != nil {
		return err
	}
	// Once etcd and kube-apiserver are stopped, the cluster will go down
	if _, important := stopImportantComponents[name]; important {
		return nil
	}
	if c.IsDryRun() {
		return nil
	}
	return c.waitComponentReady(ctx, name, false, 120*time.Second)
}

// waitComponentReady waits for a component to be ready
func (c *Cluster) waitComponentReady(ctx context.Context, name string, wantReady bool, timeout time.Duration) error {
	var (
		err     error
		waitErr error
		ready   bool
		exist   bool
	)
	logger := log.FromContext(ctx)
	waitErr = wait.Poll(ctx, func(ctx context.Context) (bool, error) {
		ready, exist, err = c.inspectComponent(ctx, name)
		if err != nil {
			logger.Debug("check component ready",
				"component", name,
				"err", err,
			)
			//nolint:nilerr
			return false, nil
		}
		if wantReady {
			return ready, nil
		}
		return !exist, nil
	},
		wait.WithTimeout(timeout),
		wait.WithImmediate(),
	)
	if err != nil {
		return err
	}
	if waitErr != nil {
		return waitErr
	}
	return nil
}

func (c *Cluster) inspectComponent(ctx context.Context, name string) (ready bool, exist bool, err error) {
	out := bytes.NewBuffer(nil)
	err = c.KubectlInCluster(exec.WithWriteTo(ctx, out), "get", "pod", "--namespace=kube-system", "--output=json", c.getComponentName(name))
	if err != nil {
		if strings.Contains(out.String(), "NotFound") {
			return false, false, nil
		}
		return false, false, err
	}

	var pod corev1.Pod
	err = json.Unmarshal(out.Bytes(), &pod)
	if err != nil {
		return false, true, err
	}

	if pod.Status.Phase != corev1.PodRunning {
		return false, true, nil
	}
	if pod.Status.ContainerStatuses == nil {
		return false, true, nil
	}
	for _, containerStatus := range pod.Status.ContainerStatuses {
		if !containerStatus.Ready {
			return false, true, nil
		}
	}

	return true, true, nil
}

func (c *Cluster) getClusterName() string {
	return c.Name() + "-control-plane"
}

func (c *Cluster) getComponentName(name string) string {
	clusterName := c.getClusterName()
	return name + "-" + clusterName
}

func (c *Cluster) logs(ctx context.Context, name string, out io.Writer, follow bool) error {
	componentName := c.getComponentName(name)

	args := []string{"logs", "-n", "kube-system"}
	if follow {
		args = append(args, "-f")
	}
	args = append(args, componentName)
	if c.IsDryRun() && !follow {
		if file, ok := dryrun.IsCatToFileWriter(out); ok {
			dryrun.PrintMessage("%s >%s", runtime.FormatExec(ctx, name, args...), file)
			return nil
		}
	}

	err := c.Kubectl(exec.WithAllWriteTo(ctx, out), args...)
	if err != nil {
		return err
	}
	return nil
}

// Logs returns the logs of the specified component.
func (c *Cluster) Logs(ctx context.Context, name string, out io.Writer) error {
	return c.logs(ctx, name, out, false)
}

// LogsFollow follows the logs of the component
func (c *Cluster) LogsFollow(ctx context.Context, name string, out io.Writer) error {
	return c.logs(ctx, name, out, true)
}

// CollectLogs returns the logs of the specified component.
func (c *Cluster) CollectLogs(ctx context.Context, dir string) error {
	logger := log.FromContext(ctx)

	kwokConfigPath := path.Join(dir, "kwok.yaml")
	if file.Exists(kwokConfigPath) {
		return fmt.Errorf("%s already exists", kwokConfigPath)
	}

	if err := c.MkdirAll(dir); err != nil {
		return fmt.Errorf("failed to create tmp directory: %w", err)
	}
	logger.Info("Exporting logs", "dir", dir)

	err := c.CopyFile(c.GetWorkdirPath(runtime.ConfigName), kwokConfigPath)
	if err != nil {
		return err
	}

	conf, err := c.Config(ctx)
	if err != nil {
		return err
	}

	componentsDir := path.Join(dir, "components")
	err = c.MkdirAll(componentsDir)
	if err != nil {
		return err
	}

	kindPath, err := c.preDownloadKind(ctx)
	if err != nil {
		return err
	}

	infoPath := path.Join(dir, conf.Options.Runtime+"-info.txt")
	err = c.WriteToPath(c.withProviderEnv(ctx), infoPath, []string{kindPath, "version"})
	if err != nil {
		return err
	}

	for _, component := range conf.Components {
		logPath := path.Join(componentsDir, component.Name+".log")
		f, err := c.OpenFile(logPath)
		if err != nil {
			logger.Error("Failed to open file", err)
			continue
		}
		if err = c.Logs(ctx, component.Name, f); err != nil {
			logger.Error("Failed to get log", err)
			if err = f.Close(); err != nil {
				logger.Error("Failed to close file", err)
				if err = c.Remove(logPath); err != nil {
					logger.Error("Failed to remove file", err)
				}
			}
		}
		if err = f.Close(); err != nil {
			logger.Error("Failed to close file", err)
			if err = c.Remove(logPath); err != nil {
				logger.Error("Failed to remove file", err)
			}
		}
	}

	if conf.Options.KubeAuditPolicy != "" {
		filePath := path.Join(componentsDir, "audit.log")
		f, err := c.OpenFile(filePath)
		if err != nil {
			logger.Error("Failed to open file", err)
		} else {
			if err = c.AuditLogs(ctx, f); err != nil {
				logger.Error("Failed to get audit log", err)
			}
			if err = f.Close(); err != nil {
				logger.Error("Failed to close file", err)
				if err = c.Remove(filePath); err != nil {
					logger.Error("Failed to remove file", err)
				}
			}
		}
	}

	return nil
}

// ListBinaries list binaries in the cluster
func (c *Cluster) ListBinaries(ctx context.Context) ([]string, error) {
	config, err := c.Config(ctx)
	if err != nil {
		return nil, err
	}
	conf := &config.Options

	return []string{
		conf.KubectlBinary,
	}, nil
}

// ListImages list images in the cluster
func (c *Cluster) ListImages(ctx context.Context) ([]string, error) {
	config, err := c.Config(ctx)
	if err != nil {
		return nil, err
	}
	conf := &config.Options

	return []string{
		conf.KindNodeImage,
		conf.KwokControllerImage,
		conf.PrometheusImage,
	}, nil
}

// EtcdctlInCluster implements the ectdctl subcommand
func (c *Cluster) EtcdctlInCluster(ctx context.Context, args ...string) error {
	etcdContainerName := c.getComponentName(consts.ComponentEtcd)

	args = append(
		[]string{
			"exec", "-i", "-n", "kube-system", etcdContainerName, "--",
			"etcdctl",
			"--endpoints=" + net.LocalAddress + ":2379",
			"--cert=/etc/kubernetes/pki/etcd/server.crt",
			"--key=/etc/kubernetes/pki/etcd/server.key",
			"--cacert=/etc/kubernetes/pki/etcd/ca.crt",
		},
		args...,
	)
	return c.KubectlInCluster(ctx, args...)
}

// preDownloadKind pre-download and cache kind
func (c *Cluster) preDownloadKind(ctx context.Context) (string, error) {
	config, err := c.Config(ctx)
	if err != nil {
		return "", err
	}
	conf := &config.Options

	_, err = exec.LookPath("kind")
	if err != nil {
		// kind does not exist, try to download it
		kindPath := c.GetBinPath("kind" + conf.BinSuffix)
		err = c.DownloadWithCache(ctx, conf.CacheDir, conf.KindBinary, kindPath, 0750, conf.QuietPull)
		if err != nil {
			return "", err
		}
		return kindPath, nil
	}

	return "kind", nil
}
