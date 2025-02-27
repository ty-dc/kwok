kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4

networking:
{{ if .KubeApiserverPort }}
  apiServerPort: {{ .KubeApiserverPort }}
{{ end }}
nodes:
- role: control-plane

  {{ if or .DashboardPort .PrometheusPort .KwokControllerPort .EtcdPort .JaegerPort}}
  extraPortMappings:
  {{ if .DashboardPort }}
  - containerPort: 8000
    hostPort: {{ .DashboardPort }}
    protocol: TCP
  {{ end }}
  {{ if .PrometheusPort }}
  - containerPort: 9090
    hostPort: {{ .PrometheusPort }}
    protocol: TCP
  {{ end }}
  {{ if .JaegerPort }}
  - containerPort: 16686
    hostPort: {{ .JaegerPort }}
    protocol: TCP
  {{ end }}
  {{ if .KwokControllerPort }}
  - containerPort: 10247
    hostPort: {{ .KwokControllerPort }}
    protocol: TCP
  {{ end }}
  {{ if .EtcdPort }}
  - containerPort: 2379
    hostPort: {{ .EtcdPort }}
    protocol: TCP
  {{ end }}
  {{ end }}

  kubeadmConfigPatches:

  {{ if or .EtcdExtraArgs .EtcdExtraVolumes }}
  - |
    kind: ClusterConfiguration
    etcd:
      local:
      {{ if .EtcdExtraArgs }}
        extraArgs:
        {{ range .EtcdExtraArgs }}
          "{{.Key}}": "{{.Value}}"
        {{ end }}
      {{ end }}

      {{ if .EtcdExtraVolumes }}
        extraVolumes:
        {{ range .EtcdExtraVolumes }}
        - name: {{ .Name }}
          hostPath: /var/components/etcd{{ .MountPath }}
          mountPath: {{ .MountPath }}
          readOnly: {{ .ReadOnly }}
          pathType: {{ .PathType }}
        {{ end }}
      {{ end }}

  {{ end }}

  {{ if or .ApiserverExtraArgs .ApiserverExtraVolumes }}
  - |
    kind: ClusterConfiguration
    apiServer:
    {{ if .ApiserverExtraArgs }}
      extraArgs:
      {{ range .ApiserverExtraArgs }}
        "{{.Key}}": "{{.Value}}"
      {{ end }}
    {{ end }}

    {{ if .ApiserverExtraVolumes }}
      extraVolumes:
      {{ range .ApiserverExtraVolumes }}
      - name: {{ .Name }}
        hostPath: /var/components/apiserver{{ .MountPath }}
        mountPath: {{ .MountPath }}
        readOnly: {{ .ReadOnly }}
        pathType: {{ .PathType }}
      {{ end }}
    {{ end }}
  {{ end }}

  {{ if or .ControllerManagerExtraArgs .ControllerManagerExtraVolumes }}
  - |
    kind: ClusterConfiguration
    controllerManager:
    {{ if .ControllerManagerExtraArgs }}
      extraArgs:
      {{ range .ControllerManagerExtraArgs }}
        "{{.Key}}": "{{.Value}}"
      {{ end }}
    {{ end }}

    {{ if .ControllerManagerExtraVolumes }}
      extraVolumes:
      {{ range .ControllerManagerExtraVolumes }}
      - name: {{ .Name }}
        hostPath: /var/components/controller-manager{{ .MountPath }}
        mountPath: {{ .MountPath }}
        readOnly: {{ .ReadOnly }}
        pathType: {{ .PathType }}
      {{ end }}
    {{ end }}
  {{ end }}

  {{ if or .SchedulerExtraArgs .SchedulerExtraVolumes }}
  - |
    kind: ClusterConfiguration
    scheduler:
    {{ if .SchedulerExtraArgs }}
      extraArgs:
      {{ range .SchedulerExtraArgs }}
        "{{.Key}}": "{{.Value}}"
      {{ end }}
    {{ end }}

    {{ if .SchedulerExtraVolumes }}
      extraVolumes:
      {{ range .SchedulerExtraVolumes }}
      - name: {{ .Name }}
        hostPath: /var/components/scheduler{{ .MountPath }}
        mountPath: {{ .MountPath }}
        readOnly: {{ .ReadOnly }}
        pathType: {{ .PathType }}
      {{ end }}
    {{ end }}
  {{ end }}

  # mount the local file on the control plane
  extraMounts:
  - hostPath: {{ .Workdir }}
    containerPath: /etc/kwok/
  - hostPath: {{ .Workdir }}/manifests
    containerPath: /etc/kubernetes/manifests
  - hostPath: {{ .Workdir }}/pki
    containerPath: /etc/kubernetes/pki

  {{ range .EtcdExtraVolumes }}
  - hostPath: {{ .HostPath }}
    containerPath: /var/components/etcd{{ .MountPath }}
    readOnly: {{ .ReadOnly }}
  {{ end }}

  {{ range .ApiserverExtraVolumes }}
  - hostPath: {{ .HostPath }}
    containerPath: /var/components/apiserver{{ .MountPath }}
    readOnly: {{ .ReadOnly }}
  {{ end }}

  {{ range .ControllerManagerExtraVolumes }}
  - hostPath: {{ .HostPath }}
    containerPath: /var/components/controller-manager{{ .MountPath }}
    readOnly: {{ .ReadOnly }}
  {{ end }}

  {{ range .SchedulerExtraVolumes }}
  - hostPath: {{ .HostPath }}
    containerPath: /var/components/scheduler{{ .MountPath }}
    readOnly: {{ .ReadOnly }}
  {{ end }}

  {{ range .KwokControllerExtraVolumes }}
  - hostPath: {{ .HostPath }}
    containerPath: /var/components/controller{{ .MountPath }}
    readOnly: {{ .ReadOnly }}
  {{ end }}

  {{ range .PrometheusExtraVolumes }}
  - hostPath: {{ .HostPath }}
    containerPath: /var/components/prometheus{{ .MountPath }}
    readOnly: {{ .ReadOnly }}
  {{ end }}

{{ if .FeatureGates }}
featureGates:
{{ range .FeatureGates }}
- {{ . }}
{{ end }}
{{ end }}

{{ if .RuntimeConfig }}
runtimeConfig:
{{ range .RuntimeConfig }}
- {{ . }}
{{ end }}
{{ end }}
