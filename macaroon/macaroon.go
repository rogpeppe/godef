package macaroon
import (
	"bytes"
	"encoding/json"
	"fmt"
	"crypto/sha256"
)

type Macaroon struct {
	location string
	rootKey []byte
	id       []byte
	caveats  []Caveat
	sig      []byte
}

type Caveat struct {
	location       string
	caveatId       []byte
	verificationId []byte
}

// New returns a new macaroon with the given identifier
func New(rootKey []byte, id, loc string) *Macaroon {
	m := &Macaroon{
		rootKey: rootKey,
		location: loc,
		id: []byte(id),
	}
	m.sig = keyedHash(m.rootKey, m.id)
	return m
}

func (m *Macaroon) Clone() *Macaroon {
	m1 := *m
	m1.caveats = make([]Caveat, len(m.caveats))
	copy(m1.caveats, m.caveats)
	return &m1
}

func (m *Macaroon) Location() string {
	return m.location
}

func (m *Macaroon) Id() []byte {
	return append([]byte(nil), m.id...)
}

func (m *Macaroon) addCaveat(caveatId, verificationId []byte, loc string) {
	m.caveats = append(m.caveats, Caveat{
		location: loc,
		caveatId: caveatId,
		verificationId: verificationId,
	})
	sig := keyedHasher(m.sig)
	sig.Write(verificationId)
	sig.Write(caveatId)
	m.sig = sig.Sum(nil)
}

func (m *Macaroon) AddFirstPersonCaveat(caveat string) {
	m.addCaveat([]byte(caveat), nil, m.location)
}

type ThirdPartyCaveat struct {
	Nonce []byte
	Caveat string
}

func (m *Macaroon) AddThirdPartyCaveat(thirdPartySecret []byte, caveat string, loc string) error {
	nonce, err := newNonce()
	if err != nil {
		return err
	}
	data, err := json.Marshal(ThirdPartyCaveat{nonce[:], caveat})
	if err != nil {
		return err
	}
	caveatId, err := encrypt(thirdPartySecret, data)
	if err != nil {
		return err
	}
	verificationId, err := encrypt(m.sig, nonce[:])
	if err != nil {
		return err
	}
	m.addCaveat(caveatId, verificationId, loc)
	return nil
}

// bndForRequest binds the given macaroon
// to the given signature of its parent macaroon.
func bindForRequest(parentSig, dischargeSig []byte) []byte {
	if len(parentSig) == 0 {
		return dischargeSig
	}
	sig := sha256.New()
	sig.Write(parentSig)
	sig.Write(dischargeSig)
	return sig.Sum(nil)
}

func (m *Macaroon) Verify(parentSig []byte, rootKey []byte, check func(caveat string) (bool, error), discharges map[string]*Macaroon) (bool, error) {
	caveatSig := keyedHash(rootKey, m.id)
	for i, cav := range m.caveats {
		if len(cav.verificationId) == 0 {
			// first-party caveat
			ok, err := check(string(cav.caveatId))
			if !ok {
				return false, err
			}
		} else {
			// third-party caveat
			cavKey, err := decrypt(caveatSig, cav.verificationId)
			if err != nil {
				return false, fmt.Errorf("failed to decrypt caveat %d signature: %v", i, err)
			}
			dm, ok := discharges[string(cav.caveatId)]
			if !ok {
				return false, fmt.Errorf("cannot find discharge macaroon for caveat %d", i)
			}
			ok, err = dm.Verify(parentSig, cavKey, check, discharges)
			if !ok {
				return false, err
			}
		}
		sig := keyedHasher(caveatSig)
		sig.Write(cav.verificationId)
		sig.Write(cav.caveatId)
		caveatSig = sig.Sum(caveatSig[:0])
	}
	// TODO perhaps we should actually do this check before doing
	// all the potentially expensive caveat checks.
	boundSig := bindForRequest(parentSig, caveatSig)
	if !bytes.Equal(boundSig, m.sig) {
		return false, fmt.Errorf("signature mismatch after caveat verification")
	}
	return true, nil
}
