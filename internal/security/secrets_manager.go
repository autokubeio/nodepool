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

package security

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	// DefaultSecretName is the default name for the credentials secret
	//nolint:gosec // G101: This is a secret name, not a hardcoded credential
	DefaultSecretName = "hcloud-credentials"
	// DefaultTokenKey is the default key for the token in the secret
	DefaultTokenKey = "token"
)

var (
	// ErrSecretNotFound indicates the secret was not found
	ErrSecretNotFound = errors.New("secret not found")
	// ErrTokenKeyNotFound indicates the token key was not found in the secret
	ErrTokenKeyNotFound = errors.New("token key not found in secret")
	// ErrEncryptionKeyRequired indicates an encryption key is required but not provided
	ErrEncryptionKeyRequired = errors.New("encryption key is required for encryption/decryption")
)

// SecretsManager manages cloud credentials stored in Kubernetes Secrets
type SecretsManager struct {
	client        kubernetes.Interface
	namespace     string
	secretName    string
	tokenKey      string
	encryptionKey []byte
}

// SecretsManagerOption is a function that configures a SecretsManager
type SecretsManagerOption func(*SecretsManager)

// WithSecretName sets a custom secret name
func WithSecretName(name string) SecretsManagerOption {
	return func(sm *SecretsManager) {
		sm.secretName = name
	}
}

// WithTokenKey sets a custom token key
func WithTokenKey(key string) SecretsManagerOption {
	return func(sm *SecretsManager) {
		sm.tokenKey = key
	}
}

// WithEncryptionKey sets the encryption key for encrypting/decrypting sensitive data
func WithEncryptionKey(key []byte) SecretsManagerOption {
	return func(sm *SecretsManager) {
		sm.encryptionKey = key
	}
}

// NewSecretsManager creates a new secrets manager
func NewSecretsManager(client kubernetes.Interface, namespace string, opts ...SecretsManagerOption) *SecretsManager {
	sm := &SecretsManager{
		client:     client,
		namespace:  namespace,
		secretName: DefaultSecretName,
		tokenKey:   DefaultTokenKey,
	}

	for _, opt := range opts {
		opt(sm)
	}

	return sm
}

// GetToken retrieves the Hetzner Cloud token from the Kubernetes secret
func (sm *SecretsManager) GetToken(ctx context.Context) (string, error) {
	secret, err := sm.client.CoreV1().Secrets(sm.namespace).Get(ctx, sm.secretName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrSecretNotFound, err)
	}

	token, ok := secret.Data[sm.tokenKey]
	if !ok {
		return "", fmt.Errorf("%w: key '%s' not found in secret '%s'", ErrTokenKeyNotFound, sm.tokenKey, sm.secretName)
	}

	return string(token), nil
}

// CreateOrUpdateSecret creates or updates the secret with the provided token
func (sm *SecretsManager) CreateOrUpdateSecret(ctx context.Context, token string) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sm.secretName,
			Namespace: sm.namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			sm.tokenKey: []byte(token),
		},
	}

	// Try to get existing secret
	existingSecret, err := sm.client.CoreV1().Secrets(sm.namespace).Get(ctx, sm.secretName, metav1.GetOptions{})
	if err != nil {
		// Secret doesn't exist, create it
		_, err = sm.client.CoreV1().Secrets(sm.namespace).Create(ctx, secret, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create secret: %w", err)
		}
		return nil
	}

	// Secret exists, update it
	existingSecret.Data = secret.Data
	_, err = sm.client.CoreV1().Secrets(sm.namespace).Update(ctx, existingSecret, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update secret: %w", err)
	}

	return nil
}

// DeleteSecret deletes the secret
func (sm *SecretsManager) DeleteSecret(ctx context.Context) error {
	err := sm.client.CoreV1().Secrets(sm.namespace).Delete(ctx, sm.secretName, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete secret: %w", err)
	}
	return nil
}

// EncryptData encrypts sensitive data using AES-GCM
func (sm *SecretsManager) EncryptData(plaintext string) (string, error) {
	if len(sm.encryptionKey) == 0 {
		return "", ErrEncryptionKeyRequired
	}

	// Ensure key is 32 bytes for AES-256
	key := make([]byte, 32)
	copy(key, sm.encryptionKey)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptData decrypts data encrypted with EncryptData
func (sm *SecretsManager) DecryptData(encryptedText string) (string, error) {
	if len(sm.encryptionKey) == 0 {
		return "", ErrEncryptionKeyRequired
	}

	// Ensure key is 32 bytes for AES-256
	key := make([]byte, 32)
	copy(key, sm.encryptionKey)

	ciphertext, err := base64.StdEncoding.DecodeString(encryptedText)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	return string(plaintext), nil
}
