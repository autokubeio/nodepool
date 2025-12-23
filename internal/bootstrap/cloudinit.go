// Package bootstrap provides cloud-init generation and token management for Kubernetes clusters.
package bootstrap

/*
Copyright 2024.

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

import (
	"bytes"
	"fmt"
	"text/template"
)

// CloudInitGenerator generates cloud-init scripts for different cluster types
type CloudInitGenerator struct {
}

// NewCloudInitGenerator creates a new cloud-init generator
func NewCloudInitGenerator() *CloudInitGenerator {
	return &CloudInitGenerator{}
}

// GenerateKubeadmCloudInit generates cloud-init for kubeadm clusters
func (g *CloudInitGenerator) GenerateKubeadmCloudInit(
	apiServerEndpoint, token, caCertHash string,
	labels map[string]string,
) (string, error) {
	return g.GenerateKubeadmCloudInitWithVersion(apiServerEndpoint, token, caCertHash, labels, "1.29")
}

// GenerateKubeadmCloudInitWithVersion generates cloud-init for kubeadm clusters with specific version
func (g *CloudInitGenerator) GenerateKubeadmCloudInitWithVersion(
	apiServerEndpoint, token, caCertHash string,
	labels map[string]string,
	k8sVersion string,
) (string, error) {
	return g.GenerateKubeadmCloudInitFull(apiServerEndpoint, token, caCertHash, labels, k8sVersion, nil, nil)
}

// GenerateKubeadmCloudInitFull generates cloud-init for kubeadm clusters with firewall and custom commands
//
//nolint:funlen,lll // Cloud-init templates require long multiline strings and complex formatting
func (g *CloudInitGenerator) GenerateKubeadmCloudInitFull(
	apiServerEndpoint, token, caCertHash string,
	_ map[string]string,
	k8sVersion string,
	firewallRules []string,
	runCmd []string,
) (string, error) {
	tmpl := `#cloud-config
package_update: true
package_upgrade: true

packages:
  - apt-transport-https
  - ca-certificates
  - curl
  - gnupg
  - ufw

runcmd:
  # Configure firewall for Kubernetes
  # - ufw --force enable
  # - ufw default deny incoming
  # - ufw default allow outgoing
  # - ufw allow ssh
  # - ufw allow 10250/tcp  # Kubelet API
  # - ufw allow 30000:32767/tcp  # NodePort Services
  # - ufw allow 8472/udp  # Flannel VXLAN
  # - ufw allow 4789/udp  # Flannel/Calico VXLAN
  # - ufw allow 179/tcp  # Calico BGP
  # - ufw allow 5473/tcp  # Calico Typha{{range .CustomFirewallRules}}
  # - ufw allow {{.}}{{end}}
  # - ufw reload

  # Setup kernel modules
  - modprobe br_netfilter
  - modprobe overlay
  - modprobe ip_vs
  - modprobe ip_vs_rr
  - modprobe ip_vs_wrr
  - modprobe ip_vs_sh
  - modprobe nf_conntrack
  - |
    cat <<EOF > /etc/modules-load.d/k8s.conf
    br_netfilter
    overlay
    ip_vs
    ip_vs_rr
    ip_vs_wrr
    ip_vs_sh
    nf_conntrack
    EOF
  
  # Setup sysctl params
  - |
    cat <<EOF > /etc/sysctl.d/k8s.conf
    net.bridge.bridge-nf-call-iptables = 1
    net.bridge.bridge-nf-call-ip6tables = 1
    net.ipv4.ip_forward = 1
    net.ipv4.conf.all.forwarding = 1
    net.ipv6.conf.all.forwarding = 1
    net.netfilter.nf_conntrack_max = 1000000
    fs.inotify.max_user_instances = 8192
    fs.inotify.max_user_watches = 524288
    vm.max_map_count = 262144
    EOF
  - sysctl --system
  
  # Disable swap
  - swapoff -a
  - sed -i '/ swap / s/^/#/' /etc/fstab
  
  # Install containerd
  - curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg  #nolint:lll
  - echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null  #nolint:lll
  - apt-get update
  - apt-get install -y containerd.io
  - mkdir -p /etc/containerd
  - containerd config default | tee /etc/containerd/config.toml
  - sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml
  - systemctl restart containerd
  - systemctl enable containerd
  
  # Install kubeadm, kubelet, kubectl (version {{.K8sVersion}})
  - curl -fsSL https://pkgs.k8s.io/core:/stable:/v{{.K8sVersion}}/deb/Release.key | gpg --dearmor -o /usr/share/keyrings/kubernetes-archive-keyring.gpg  #nolint:lll
  - echo "deb [signed-by=/usr/share/keyrings/kubernetes-archive-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v{{.K8sVersion}}/deb/ /" | tee /etc/apt/sources.list.d/kubernetes.list  #nolint:lll
  - apt-get update
  - apt-get install -y kubelet kubeadm kubectl
  - apt-mark hold kubelet kubeadm kubectl
  
  # Configure kubelet
  - |
    cat <<EOF > /etc/default/kubelet
    KUBELET_EXTRA_ARGS=--node-ip=$(hostname -I | awk '{print $1}')
    EOF
  - systemctl daemon-reload
  - systemctl enable kubelet
  
  # Join cluster with token
  - |
    kubeadm join {{.APIServerEndpoint}} \
      --token {{.Token}} \
      --discovery-token-ca-cert-hash {{.CACertHash}} \
      --v=5
{{range .RunCmd}}
  # User command
  - {{.}}{{end}}

write_files:
  - path: /etc/crictl.yaml
    content: |
      runtime-endpoint: unix:///run/containerd/containerd.sock
      image-endpoint: unix:///run/containerd/containerd.sock
      timeout: 10

power_state:
  mode: reboot
  condition: True
`

	config := struct {
		APIServerEndpoint   string
		Token               string
		CACertHash          string
		K8sVersion          string
		CustomFirewallRules []string
		RunCmd              []string
	}{
		APIServerEndpoint:   apiServerEndpoint,
		Token:               token,
		CACertHash:          caCertHash,
		K8sVersion:          k8sVersion,
		CustomFirewallRules: firewallRules,
		RunCmd:              runCmd,
	}

	t, err := template.New("kubeadm").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, config); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// GenerateK3sCloudInit generates cloud-init for k3s clusters
func (g *CloudInitGenerator) GenerateK3sCloudInit(serverURL, token string, labels map[string]string) (string, error) {
	tmpl := `#cloud-config
package_update: true
package_upgrade: true

write_files:
  - path: /etc/rancher/k3s/config.yaml
    content: |
      server: {{.ServerURL}}
      token: {{.Token}}
      {{range $k, $v := .Labels}}
      node-label:
        - "{{$k}}={{$v}}"
      {{end}}

runcmd:
  # Install k3s agent
  - |
    curl -sfL https://get.k3s.io | sh -s - agent
  # Wait for k3s to be ready
  - until kubectl get nodes; do sleep 5; done
`

	config := struct {
		ServerURL string
		Token     string
		Labels    map[string]string
	}{
		ServerURL: serverURL,
		Token:     token,
		Labels:    labels,
	}

	t, err := template.New("k3s").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, config); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// GenerateTalosCloudInit generates cloud-init for Talos clusters
// Note: Talos doesn't use cloud-init but machine configs
func (g *CloudInitGenerator) GenerateTalosCloudInit(controlPlaneEndpoint, machineConfig string) (string, error) {
	// Talos doesn't use cloud-init, it uses machine config
	// This would be applied differently
	return fmt.Sprintf(`# Talos Machine Config
# Apply this with: talosctl apply-config --insecure --nodes <node-ip> --file <this-file>
machine:
  type: worker
  token: %s
  controlPlane:
    endpoint: %s
`, machineConfig, controlPlaneEndpoint), nil
}

// GenerateRancherCloudInit generates cloud-init for Rancher/RKE2 clusters
func (g *CloudInitGenerator) GenerateRancherCloudInit(
	serverURL, token string,
	labels map[string]string,
) (string, error) {
	tmpl := `#cloud-config
package_update: true
package_upgrade: true

runcmd:
  # Install RKE2 agent
  - curl -sfL https://get.rke2.io | INSTALL_RKE2_TYPE="agent" sh -
  
  # Configure RKE2
  - mkdir -p /etc/rancher/rke2/
  - |
    cat > /etc/rancher/rke2/config.yaml <<EOF
    server: {{.ServerURL}}
    token: {{.Token}}
    {{range $k, $v := .Labels}}
    node-label:
      - "{{$k}}={{$v}}"
    {{end}}
    EOF
  
  # Start RKE2 agent
  - systemctl enable rke2-agent.service
  - systemctl start rke2-agent.service
`

	config := struct {
		ServerURL string
		Token     string
		Labels    map[string]string
	}{
		ServerURL: serverURL,
		Token:     token,
		Labels:    labels,
	}

	t, err := template.New("rke2").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, config); err != nil {
		return "", err
	}

	return buf.String(), nil
}
