// SPDX-License-Identifier: AGPL-3.0-or-later

package http

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"

	"github.com/kolapsis/shm/internal/app"
	"github.com/kolapsis/shm/pkg/crypto"
)

// KeyProvider is an interface for retrieving public keys for signature verification.
type KeyProvider interface {
	GetPublicKey(ctx context.Context, instanceID string) (string, error)
}

// AuthMiddleware provides Ed25519 signature verification for requests.
type AuthMiddleware struct {
	keys   KeyProvider
	logger *slog.Logger
}

// NewAuthMiddleware creates a new AuthMiddleware.
func NewAuthMiddleware(keys KeyProvider, logger *slog.Logger) *AuthMiddleware {
	if logger == nil {
		logger = slog.Default()
	}
	return &AuthMiddleware{
		keys:   keys,
		logger: logger,
	}
}

// NewAuthMiddlewareFromService creates an AuthMiddleware using an InstanceService.
func NewAuthMiddlewareFromService(svc *app.InstanceService, logger *slog.Logger) *AuthMiddleware {
	return NewAuthMiddleware(&instanceServiceKeyProvider{svc: svc}, logger)
}

// instanceServiceKeyProvider adapts InstanceService to KeyProvider.
type instanceServiceKeyProvider struct {
	svc *app.InstanceService
}

func (p *instanceServiceKeyProvider) GetPublicKey(ctx context.Context, instanceID string) (string, error) {
	return p.svc.GetPublicKey(ctx, instanceID)
}

// RequireSignature wraps a handler to require a valid Ed25519 signature.
// The request must have X-Instance-ID and X-Signature headers.
// The signature is verified against the request body using the instance's public key.
func (m *AuthMiddleware) RequireSignature(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		instanceID := r.Header.Get("X-Instance-ID")
		signature := r.Header.Get("X-Signature")

		m.logger.Debug("auth attempt", "instance_id", instanceID)

		if instanceID == "" || signature == "" {
			m.logger.Warn("missing auth headers",
				"instance_id", instanceID,
				"has_signature", signature != "",
			)
			http.Error(w, "Missing authentication headers", http.StatusUnauthorized)
			return
		}

		// Read and buffer the body for verification
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			m.logger.Error("failed to read body", "instance_id", instanceID, "error", err)
			http.Error(w, "Failed to read request body", http.StatusInternalServerError)
			return
		}
		// Restore the body for downstream handlers
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		// Get the public key for this instance
		pubKey, err := m.keys.GetPublicKey(r.Context(), instanceID)
		if err != nil {
			m.logger.Warn("key lookup failed", "instance_id", instanceID, "error", err)
			http.Error(w, "Unauthorized", http.StatusForbidden)
			return
		}

		// Verify the signature
		if !crypto.Verify(pubKey, bodyBytes, signature) {
			m.logger.Warn("invalid signature", "instance_id", instanceID)
			http.Error(w, "Invalid signature", http.StatusForbidden)
			return
		}

		m.logger.Debug("auth success", "instance_id", instanceID)
		next(w, r)
	}
}
