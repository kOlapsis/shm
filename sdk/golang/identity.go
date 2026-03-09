// SPDX-License-Identifier: MIT

package golang

import (
	"encoding/hex"
	"encoding/json"
	"os"

	"github.com/google/uuid"
	"github.com/kolapsis/shm/pkg/crypto"
)

type Identity struct {
	InstanceID string `json:"instance_id"`
	PrivateKey string `json:"private_key"` // Hex encoded
	PublicKey  string `json:"public_key"`  // Hex encoded
}

func loadOrGenerateIdentity(filePath string) (*Identity, error) {
	if _, err := os.Stat(filePath); err == nil {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, err
		}
		var id Identity
		if err := json.Unmarshal(data, &id); err == nil {
			return &id, nil
		}
	}

	pub, priv, err := crypto.GenerateKeypair()
	if err != nil {
		return nil, err
	}

	id := &Identity{
		InstanceID: uuid.New().String(),
		PrivateKey: hex.EncodeToString(priv),
		PublicKey:  hex.EncodeToString(pub),
	}

	data, _ := json.MarshalIndent(id, "", "  ")
	if err := os.WriteFile(filePath, data, 0600); err != nil {
		return nil, err
	}

	return id, nil
}
