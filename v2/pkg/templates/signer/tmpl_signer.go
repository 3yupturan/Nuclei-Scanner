package signer

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/projectdiscovery/gologger"
	"github.com/projectdiscovery/nuclei/v2/pkg/catalog/config"
	errorutil "github.com/projectdiscovery/utils/errors"
)

var (
	ReDigest            = regexp.MustCompile(`(?m)^#\sdigest:\s.+$`)
	ErrUnknownAlgorithm = errors.New("unknown algorithm")
	SignaturePattern    = "# digest: "
	SignatureFmt        = SignaturePattern + "%x"
)

func RemoveSignatureFromData(data []byte) []byte {
	return bytes.Trim(ReDigest.ReplaceAll(data, []byte("")), "\n")
}

func GetSignatureFromData(data []byte) []byte {
	return ReDigest.Find(data)
}

// SignableTemplate is a template that can be signed
type SignableTemplate interface {
	// GetFileImports returns a list of files that are imported by the template
	GetFileImports() []string
	// HasCodeProtocol returns true if the template has a code protocol section
	HasCodeProtocol() bool
}

type TemplateSigner struct {
	handler *KeyHandler
}

// Identifier returns the identifier for the template signer
func (t *TemplateSigner) Identifier() string {
	return t.handler.cert.Subject.CommonName
}

// fragment is a digital signature of public key using the private key
// it is used to identify the signer
func (t *TemplateSigner) GetUserFragment() (string, error) {
	hashed := sha256.Sum256(t.handler.ecdsaPubKey.X.Bytes())
	ecdsaSignature, err := ecdsa.SignASN1(rand.Reader, t.handler.ecdsaKey, hashed[:])
	if err != nil {
		return "", err
	}
	var signatureData bytes.Buffer
	if err := gob.NewEncoder(&signatureData).Encode(ecdsaSignature); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", signatureData.String()), nil
}

// Sign signs the given template with the template signer and returns the signature
func (t *TemplateSigner) Sign(data []byte, tmpl SignableTemplate) (string, error) {
	// while re-signing template check if it has a code protocol
	// if it does then verify that it is signed by current signer
	// if not then return error
	if tmpl.HasCodeProtocol() {
		sig := GetSignatureFromData(data)
		arr := strings.SplitN(string(sig), "@", 2)
		if len(arr) == 1 && strings.TrimSpace(arr[0]) != "" {
			// signature has no fragment
			return "", errorutil.NewWithTag("signer", "re-signing code protocol templates not allowed: no fragment found")
		}
		if len(arr) == 2 {
			fragment, _ := t.GetUserFragment()
			if fragment != arr[1] {
				fmt.Printf("signature: %v\n", arr[0])
				fmt.Printf("existing fragment: %v\n", arr[1])
				fmt.Printf("current fragment: %v\n", fragment)
				return "", errorutil.NewWithTag("signer", "fragment mismatch: expected '%v' got '%v'", fragment, arr[1])
			}
		}
	}

	buff := bytes.NewBuffer(RemoveSignatureFromData(data))
	// if file has any imports process them
	for _, file := range tmpl.GetFileImports() {
		bin, err := os.ReadFile(file)
		if err != nil {
			return "", err
		}
		buff.WriteRune('\n')
		buff.Write(bin)
	}
	signatureData, err := t.sign(buff.Bytes())
	if err != nil {
		return "", err
	}
	return signatureData, nil
}

// Signs given data with the template signer
// Note: this should not be used for signing templates as file references
// in templates are not processed use template.SignTemplate() instead
func (t *TemplateSigner) sign(data []byte) (string, error) {
	dataHash := sha256.Sum256(data)
	ecdsaSignature, err := ecdsa.SignASN1(rand.Reader, t.handler.ecdsaKey, dataHash[:])
	if err != nil {
		return "", err
	}
	var signatureData bytes.Buffer
	if err := gob.NewEncoder(&signatureData).Encode(ecdsaSignature); err != nil {
		return "", err
	}
	// add fragment of user along with signature
	fragment, err := t.GetUserFragment()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(SignatureFmt, signatureData.Bytes()) + "@" + fragment, nil
}

// Verify verifies the given template with the template signer
func (t *TemplateSigner) Verify(data []byte, tmpl SignableTemplate) (bool, error) {
	digestData := ReDigest.Find(data)
	if len(digestData) == 0 {
		return false, errors.New("digest not found")
	}

	digestData = bytes.TrimSpace(bytes.TrimPrefix(digestData, []byte(SignaturePattern)))
	// trim fragment if any
	signature := strings.SplitN(string(digestData), "@", 2)
	if len(signature) == 1 && strings.TrimSpace(signature[0]) == "" {
		// no signature found hence not verified
		return false, nil
	}
	digest, err := hex.DecodeString(signature[0])
	if err != nil {
		return false, err
	}

	buff := bytes.NewBuffer(RemoveSignatureFromData(data))
	// if file has any imports process them
	for _, file := range tmpl.GetFileImports() {
		bin, err := os.ReadFile(file)
		if err != nil {
			return false, err
		}
		buff.WriteRune('\n')
		buff.Write(bin)
	}

	return t.verify(buff.Bytes(), digest)
}

// Verify verifies the given data with the template signer
// Note: this should not be used for verifying templates as file references
// in templates are not processed
func (t *TemplateSigner) verify(data, signatureData []byte) (bool, error) {
	dataHash := sha256.Sum256(data)

	var signature []byte
	if err := gob.NewDecoder(bytes.NewReader(signatureData)).Decode(&signature); err != nil {
		return false, err
	}
	return ecdsa.VerifyASN1(t.handler.ecdsaPubKey, dataHash[:], signature), nil
}

// NewTemplateSigner creates a new signer for signing templates
func NewTemplateSigner(cert, privateKey []byte) (*TemplateSigner, error) {
	handler := &KeyHandler{}
	var err error
	if cert != nil || privateKey != nil {
		handler.UserCert = cert
		handler.PrivateKey = privateKey
	} else {
		err = handler.ReadCert(CertEnvVarName, config.DefaultConfig.GetKeysDir())
		if err == nil {
			err = handler.ReadPrivateKey(PrivateKeyEnvName, config.DefaultConfig.GetKeysDir())
		}
	}
	if err != nil && !SkipGeneratingKeys {
		if err != ErrNoCertificate && err != ErrNoPrivateKey {
			gologger.Info().Msgf("Invalid user cert found : %s\n", err)
		}
		// generating new keys
		handler.GenerateKeyPair()
		if err := handler.SaveToDisk(config.DefaultConfig.GetKeysDir()); err != nil {
			gologger.Fatal().Msgf("could not save generated keys to disk: %s\n", err)
		}
		// do not continue further let user re-run the command
		os.Exit(0)
	} else if err != nil && SkipGeneratingKeys {
		return nil, err
	}

	if err := handler.ParseUserCert(); err != nil {
		return nil, err
	}
	if err := handler.ParsePrivateKey(); err != nil {
		return nil, err
	}
	return &TemplateSigner{
		handler: handler,
	}, nil
}

// NewTemplateSignerFromFiles creates a new signer for signing templates
func NewTemplateSignerFromFiles(cert, privKey string) (*TemplateSigner, error) {
	certData, err := os.ReadFile(cert)
	if err != nil {
		return nil, err
	}
	privKeyData, err := os.ReadFile(privKey)
	if err != nil {
		return nil, err
	}
	return NewTemplateSigner(certData, privKeyData)
}

// NewTemplateSigVerifier creates a new signer for verifying templates
func NewTemplateSigVerifier(cert []byte) (*TemplateSigner, error) {
	handler := &KeyHandler{}
	if cert != nil {
		handler.UserCert = cert
	} else {
		if err := handler.ReadCert(CertEnvVarName, config.DefaultConfig.GetKeysDir()); err != nil {
			return nil, err
		}
	}
	if err := handler.ParseUserCert(); err != nil {
		return nil, err
	}
	return &TemplateSigner{
		handler: handler,
	}, nil
}