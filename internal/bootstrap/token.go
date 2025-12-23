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
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// BootstrapTokenManager manages Kubernetes bootstrap tokens
//
//nolint:revive // Keeping existing type name for backward compatibility
type BootstrapTokenManager struct {
	client kubernetes.Interface
}

// BootstrapToken represents a bootstrap token with its metadata
//
//nolint:revive // Keeping existing type name for backward compatibility
type BootstrapToken struct {
	Token     string
	TokenID   string
	ExpiresAt time.Time
}

// ClusterInfo contains information about the cluster
type ClusterInfo struct {
	Endpoint   string
	CACertHash string
}

// NewBootstrapTokenManager creates a new bootstrap token manager
func NewBootstrapTokenManager(client kubernetes.Interface) *BootstrapTokenManager {
	return &BootstrapTokenManager{
		client: client,
	}
}

// GetOrGenerateBootstrapToken gets an existing valid token or creates a new one
func (m *BootstrapTokenManager) GetOrGenerateBootstrapToken(
	ctx context.Context,
	name string,
	duration time.Duration,
) (*BootstrapToken, error) {
	// Check for existing valid token
	secrets, err := m.client.CoreV1().Secrets("kube-system").List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("managed-by=nodepools,nodepool=%s", name),
	})
	if err == nil && len(secrets.Items) > 0 {
		// Use the first valid token found
		for _, secret := range secrets.Items {
			if expirationStr, ok := secret.Data["expiration"]; ok {
				expiration, err := time.Parse(time.RFC3339, string(expirationStr))
				if err == nil && time.Until(expiration) > 1*time.Hour {
					// Token is still valid for at least 1 hour
					tokenID := string(secret.Data["token-id"])
					tokenSecret := string(secret.Data["token-secret"])
					return &BootstrapToken{
						Token:     fmt.Sprintf("%s.%s", tokenID, tokenSecret),
						TokenID:   tokenID,
						ExpiresAt: expiration,
					}, nil
				}
			}
		}
	}

	// No valid token found, generate new one
	return m.GenerateBootstrapToken(ctx, name, duration)
}

// GenerateBootstrapToken creates a new bootstrap token with specified duration
func (m *BootstrapTokenManager) GenerateBootstrapToken(
	ctx context.Context,
	name string,
	duration time.Duration,
) (*BootstrapToken, error) {
	// Generate random token ID and secret
	tokenID := generateRandomString(6)
	tokenSecret := generateRandomString(16)

	// Create bootstrap token secret
	expiresAt := time.Now().Add(duration)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("bootstrap-token-%s", tokenID),
			Namespace: "kube-system",
			Labels: map[string]string{
				"managed-by": "nodepools",
				"nodepool":   name,
			},
		},
		Type: corev1.SecretTypeBootstrapToken,
		StringData: map[string]string{
			"token-id":                       tokenID,
			"token-secret":                   tokenSecret,
			"expiration":                     expiresAt.Format(time.RFC3339),
			"usage-bootstrap-authentication": "true",
			"usage-bootstrap-signing":        "true",
			"auth-extra-groups":              "system:bootstrappers:kubeadm:default-node-token",
		},
	}

	_, err := m.client.CoreV1().Secrets("kube-system").Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create bootstrap token secret: %w", err)
	}

	return &BootstrapToken{
		Token:     fmt.Sprintf("%s.%s", tokenID, tokenSecret),
		TokenID:   tokenID,
		ExpiresAt: expiresAt,
	}, nil
}

// GetClusterInfo retrieves cluster endpoint and CA certificate hash
func (m *BootstrapTokenManager) GetClusterInfo(ctx context.Context) (*ClusterInfo, error) {
	// Get cluster-info configmap
	cm, err := m.client.CoreV1().ConfigMaps("kube-public").Get(ctx, "cluster-info", metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster-info configmap: %w", err)
	}

	kubeconfig, ok := cm.Data["kubeconfig"]
	if !ok {
		return nil, fmt.Errorf("kubeconfig not found in cluster-info")
	}

	// Parse endpoint from kubeconfig
	endpoint := extractServerFromKubeconfig(kubeconfig)

	// Extract CA certificate from kubeconfig
	caCertBase64 := extractCACertFromKubeconfig(kubeconfig)
	if caCertBase64 == "" {
		return nil, fmt.Errorf("CA certificate not found in cluster-info")
	}

	// Decode base64 CA certificate
	caCert, err := base64.StdEncoding.DecodeString(caCertBase64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode CA certificate: %w", err)
	}

	// Calculate CA cert hash
	caCertHash := calculateCACertHash(caCert)

	return &ClusterInfo{
		Endpoint:   endpoint,
		CACertHash: fmt.Sprintf("sha256:%s", caCertHash),
	}, nil
}

// DeleteBootstrapToken removes a bootstrap token
func (m *BootstrapTokenManager) DeleteBootstrapToken(ctx context.Context, tokenID string) error {
	secretName := fmt.Sprintf("bootstrap-token-%s", tokenID)
	err := m.client.CoreV1().Secrets("kube-system").Delete(ctx, secretName, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete bootstrap token: %w", err)
	}
	return nil
}

// generateRandomString generates a random string of specified length
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[n.Int64()]
	}
	return string(b)
}

// calculateCACertHash calculates the SHA256 hash of the CA certificate public key
func calculateCACertHash(caCert []byte) string {
	// Parse the PEM-encoded certificate
	block, _ := pem.Decode(caCert)
	if block == nil {
		return ""
	}

	// Parse the certificate
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return ""
	}

	// Marshal the public key to DER format
	pubKeyDER, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		return ""
	}

	// Calculate SHA256 hash of the public key
	hash := sha256.Sum256(pubKeyDER)
	return hex.EncodeToString(hash[:])
}

// extractServerFromKubeconfig extracts the server URL from kubeconfig
func extractServerFromKubeconfig(kubeconfig string) string {
	const serverPrefix = "server: "
	start := 0
	for {
		idx := findInString(kubeconfig[start:], serverPrefix)
		if idx == -1 {
			return ""
		}
		start += idx + len(serverPrefix)
		end := findInString(kubeconfig[start:], "\n")
		if end == -1 {
			end = len(kubeconfig) - start
		}
		endpoint := kubeconfig[start : start+end]
		// Remove https:// or http:// prefix for kubeadm
		if len(endpoint) > 8 && endpoint[:8] == "https://" {
			return endpoint[8:]
		}
		if len(endpoint) > 7 && endpoint[:7] == "http://" {
			return endpoint[7:]
		}
		// If no prefix, return as-is
		if endpoint != "" {
			return endpoint
		}
	}
}

// extractCACertFromKubeconfig extracts the certificate-authority-data from kubeconfig
func extractCACertFromKubeconfig(kubeconfig string) string {
	const caPrefix = "certificate-authority-data: "
	start := 0
	idx := findInString(kubeconfig[start:], caPrefix)
	if idx == -1 {
		return ""
	}
	start = idx + len(caPrefix)
	end := findInString(kubeconfig[start:], "\n")
	if end == -1 {
		end = len(kubeconfig) - start
	}
	return kubeconfig[start : start+end]
}

// findInString is a helper to find substring
func findInString(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
