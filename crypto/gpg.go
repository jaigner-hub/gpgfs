// Package crypto provides GPG-based encryption and decryption functionality.
package crypto

import (
	"bytes"
	"errors"
	"io"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
)

var (
	ErrNoPrivateKey     = errors.New("no private key available")
	ErrDecryptionFailed = errors.New("decryption failed")
	ErrEncryptionFailed = errors.New("encryption failed")
)

// GPGHandler manages GPG encryption and decryption operations.
type GPGHandler struct {
	entity     *openpgp.Entity
	passphrase []byte
}

// NewGPGHandler creates a new GPG handler with the given passphrase.
// It generates a new key pair for encryption/decryption.
func NewGPGHandler(passphrase string) (*GPGHandler, error) {
	config := &packet.Config{
		DefaultCipher: packet.CipherAES256,
	}

	entity, err := openpgp.NewEntity("gpgfs", "GPGFS Encrypted Filesystem", "", config)
	if err != nil {
		return nil, err
	}

	return &GPGHandler{
		entity:     entity,
		passphrase: []byte(passphrase),
	}, nil
}

// NewGPGHandlerFromKey creates a GPG handler from an existing armored private key.
func NewGPGHandlerFromKey(armoredKey string, passphrase string) (*GPGHandler, error) {
	block, err := armor.Decode(bytes.NewBufferString(armoredKey))
	if err != nil {
		return nil, err
	}

	entityList, err := openpgp.ReadKeyRing(block.Body)
	if err != nil {
		return nil, err
	}

	if len(entityList) == 0 {
		return nil, ErrNoPrivateKey
	}

	return &GPGHandler{
		entity:     entityList[0],
		passphrase: []byte(passphrase),
	}, nil
}

// ExportPublicKey exports the public key in armored format.
func (g *GPGHandler) ExportPublicKey() (string, error) {
	var buf bytes.Buffer
	w, err := armor.Encode(&buf, openpgp.PublicKeyType, nil)
	if err != nil {
		return "", err
	}

	if err := g.entity.Serialize(w); err != nil {
		w.Close()
		return "", err
	}
	w.Close()

	return buf.String(), nil
}

// ExportPrivateKey exports the private key in armored format.
func (g *GPGHandler) ExportPrivateKey() (string, error) {
	var buf bytes.Buffer
	w, err := armor.Encode(&buf, openpgp.PrivateKeyType, nil)
	if err != nil {
		return "", err
	}

	if err := g.entity.SerializePrivate(w, nil); err != nil {
		w.Close()
		return "", err
	}
	w.Close()

	return buf.String(), nil
}

// Encrypt encrypts the given plaintext data.
func (g *GPGHandler) Encrypt(plaintext []byte) ([]byte, error) {
	var buf bytes.Buffer

	// Create encryption writer
	w, err := openpgp.Encrypt(&buf, []*openpgp.Entity{g.entity}, nil, nil, nil)
	if err != nil {
		return nil, err
	}

	_, err = w.Write(plaintext)
	if err != nil {
		w.Close()
		return nil, err
	}

	if err := w.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// Decrypt decrypts the given ciphertext data.
func (g *GPGHandler) Decrypt(ciphertext []byte) ([]byte, error) {
	// Decrypt private key if needed
	if g.entity.PrivateKey != nil && g.entity.PrivateKey.Encrypted {
		if err := g.entity.PrivateKey.Decrypt(g.passphrase); err != nil {
			return nil, err
		}
	}

	for _, subkey := range g.entity.Subkeys {
		if subkey.PrivateKey != nil && subkey.PrivateKey.Encrypted {
			if err := subkey.PrivateKey.Decrypt(g.passphrase); err != nil {
				return nil, err
			}
		}
	}

	entityList := openpgp.EntityList{g.entity}

	md, err := openpgp.ReadMessage(bytes.NewReader(ciphertext), entityList, nil, nil)
	if err != nil {
		return nil, err
	}

	plaintext, err := io.ReadAll(md.UnverifiedBody)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// EncryptStream returns a writer that encrypts data as it's written.
func (g *GPGHandler) EncryptStream(w io.Writer) (io.WriteCloser, error) {
	return openpgp.Encrypt(w, []*openpgp.Entity{g.entity}, nil, nil, nil)
}
