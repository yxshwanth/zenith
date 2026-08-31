// Package token encodes opaque revision tokens (docs/adr/0003-logical-revision-as-clock.md).
// Format is not a Cockroach zookie and must not be cast from int64 timestamps.
package token

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
)

const formatVersion byte = 1

// Claims are the validated fields inside a token.
type Claims struct {
	Epoch    uint64
	Group    uint64
	Store    string
	Revision uint64
}

// Codec mints and verifies tokens bound to a cluster secret.
type Codec struct {
	Secret []byte
	Epoch  uint64
	Group  uint64
}

var (
	ErrMalformed  = errors.New("token: malformed")
	ErrTampered   = errors.New("token: tampered")
	ErrWrongEpoch = errors.New("token: wrong epoch")
	ErrWrongGroup = errors.New("token: wrong group")
	ErrWrongStore = errors.New("token: wrong store")
)

// Mint returns an opaque string for store at revision.
func (c *Codec) Mint(store string, revision uint64) string {
	payload := encodePayload(formatVersion, c.Epoch, c.Group, store, revision)
	mac := mac(c.Secret, payload)
	raw := append(payload, mac...)
	return base64.RawURLEncoding.EncodeToString(raw)
}

// Verify parses tok and checks epoch/group/store (if expectStore != "").
func (c *Codec) Verify(tok, expectStore string) (Claims, error) {
	raw, err := base64.RawURLEncoding.DecodeString(tok)
	if err != nil || len(raw) < 1+8+8+4+8+32 {
		return Claims{}, ErrMalformed
	}
	macGot := raw[len(raw)-32:]
	payload := raw[:len(raw)-32]
	if !hmac.Equal(mac(c.Secret, payload), macGot) {
		return Claims{}, ErrTampered
	}
	cl, err := decodePayload(payload)
	if err != nil {
		return Claims{}, err
	}
	if cl.Epoch != c.Epoch {
		return Claims{}, ErrWrongEpoch
	}
	if cl.Group != c.Group {
		return Claims{}, ErrWrongGroup
	}
	if expectStore != "" && cl.Store != expectStore {
		return Claims{}, ErrWrongStore
	}
	return cl, nil
}

func encodePayload(ver byte, epoch, group uint64, store string, rev uint64) []byte {
	sb := []byte(store)
	b := make([]byte, 0, 1+8+8+4+len(sb)+8)
	b = append(b, ver)
	b = binary.BigEndian.AppendUint64(b, epoch)
	b = binary.BigEndian.AppendUint64(b, group)
	b = binary.BigEndian.AppendUint32(b, uint32(len(sb)))
	b = append(b, sb...)
	b = binary.BigEndian.AppendUint64(b, rev)
	return b
}

func decodePayload(b []byte) (Claims, error) {
	if len(b) < 1+8+8+4+8 {
		return Claims{}, ErrMalformed
	}
	if b[0] != formatVersion {
		return Claims{}, fmt.Errorf("%w: version", ErrMalformed)
	}
	i := 1
	epoch := binary.BigEndian.Uint64(b[i : i+8])
	i += 8
	group := binary.BigEndian.Uint64(b[i : i+8])
	i += 8
	n := int(binary.BigEndian.Uint32(b[i : i+4]))
	i += 4
	if n < 0 || i+n+8 > len(b) {
		return Claims{}, ErrMalformed
	}
	store := string(b[i : i+n])
	i += n
	rev := binary.BigEndian.Uint64(b[i : i+8])
	return Claims{Epoch: epoch, Group: group, Store: store, Revision: rev}, nil
}

func mac(secret, payload []byte) []byte {
	m := hmac.New(sha256.New, secret)
	m.Write(payload)
	return m.Sum(nil)
}
