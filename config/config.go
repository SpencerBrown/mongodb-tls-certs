package config

import (
	"crypto"
	"crypto/x509"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
)

// defaults for directories and extensions
const (
	defaultPublicDirectory  = "tls"
	defaultPrivateDirectory = "tls/private"
	defaultExtensionKey     = "key"
	defaultExtensionCert    = "pem"
)

// Certificate types as enums
const (
	RootCACert = iota
	IntermediateCACert
	OCSPSigningCert
	ServerCert
	ClientCert
)

// information about a certificate
type certInfo struct {
	ctype        int  // certificate type
	isCA         bool // is it a CA?
	isSelfSigned bool // is it self-signed?
}

// getCertTYpe converts a type string to information about the kind of certificate it is
func getCertType(typeString string) (*certInfo, error) {
	theMap := map[string]certInfo{
		"rootCA":         {RootCACert, true, true},
		"intermediateCA": {IntermediateCACert, true, false},
		"OCSPSigning":    {OCSPSigningCert, false, false},
		"server":         {ServerCert, false, false},
		"client":         {ClientCert, false, false},
	}
	certType, ok := theMap[typeString]
	if ok {
		return &certType, nil
	} else {
		return nil, fmt.Errorf("invalid certificate type '%s'", typeString)
	}
}

// SubjectType is the type for a subject name
type SubjectType struct {
	O  string `yaml:"O"`
	OU string `yaml:"OU"`
	CN string `yaml:"CN"`
}

// Cert type is an Internal representation of a certificate specification,
// some filled in from the YAML config file, some calculated
type Cert struct {
	// Filled in by YAML unmarshalling
	TypeString string `yaml:"type"`
	Issuer     string `yaml:"issuer"`
	Subject    SubjectType
	Hosts      []string `yaml:"hosts"`
	// Created programmatically
	Type         int               `yaml:"-"`
	IsCA         bool              `yaml:"-"`
	IsSelfSigned bool              `yaml:"-"`
	PrivateKey   crypto.PrivateKey `yaml:"-"`
	Certificate  *x509.Certificate `yaml:"-"`
	IssuerCert   *Cert             `yaml:"-"`
}

// Type type is the internal representation of the entire config file,
// some filled in from the YAML config files, some calculated
type Type struct {
	// filled in by YAML unmarshalling
	Directories  map[string]string `yaml:"directories"`
	Extensions   map[string]string `yaml:"extensions"`
	Subject      SubjectType
	KeyFiles     []string            `yaml:"keyfiles"`
	Certificates map[string]*Cert    `yaml:"certificates"`
	Combos       map[string][]string `yaml:"combos"`
	// filled in programmatically
	PublicDirectory  string `yaml:"-"`
	PrivateDirectory string `yaml:"-"`
	ExtensionKey     string `yaml:"-"`
	ExtensionCert    string `yaml:"-"`
}

// Config is a global variable for "THE CONFIG", there will only be one per run
var Config Type

// GetConfig is responsible for parsing the YAML file and filling in the global variable Config
func GetConfig(configFilename *string) error {
	configFile, err := os.ReadFile(filepath.Clean(*configFilename))
	if err != nil {
		return fmt.Errorf("error reading config file '%s': %v", *configFilename, err)
	}
	err = yaml.Unmarshal(configFile, &Config)
	if err != nil {
		return fmt.Errorf("error parsing YAML config file '%s': %v", *configFilename, err)
	}

	Config.PublicDirectory = defaultPublicDirectory
	Config.PrivateDirectory = defaultPrivateDirectory
	if Config.Directories != nil {
		for dirName, dir := range Config.Directories {
			switch dirName {
			case "public":
				Config.PublicDirectory = dir
			case "private":
				Config.PrivateDirectory = dir
			default:
				return fmt.Errorf("invalid entry %s in directories section of config file %s", dirName, *configFilename)
			}
		}
	}

	Config.ExtensionKey = defaultExtensionKey
	Config.ExtensionCert = defaultExtensionCert
	if Config.Extensions != nil {
		for extName, ext := range Config.Extensions {
			switch extName {
			case "key":
				Config.ExtensionKey = ext
			case "certificate":
				Config.ExtensionCert = ext
			default:
				return fmt.Errorf("invalid entry %s in extensions section of config file %s", extName, ext)
			}
		}
	}

	// Do some setup on the certificate configurations
	// - fill in the Type, IsSelfSigned, and  IsCA field for each certificate
	// - fill in default subject fields
	// - make sure self-signed certs don't have issuer
	// - fill in issuer pointer for cert's issuer
	// - make sure issuer-signed certs have an issuer that is a CA
	for certName, cert := range Config.Certificates {
		certType, err := getCertType(cert.TypeString)
		if err != nil {
			return fmt.Errorf("invalid type %s for certificate %s", certName, cert.TypeString)
		}
		cert.Type = certType.ctype
		cert.IsCA = certType.isCA
		cert.IsSelfSigned = certType.isSelfSigned
		if cert.Subject.O == "" {
			cert.Subject.O = Config.Subject.O
		}
		if cert.Subject.OU == "" {
			cert.Subject.OU = Config.Subject.OU
		}
		if cert.Subject.CN == "" {
			cert.Subject.CN = Config.Subject.CN
		}
	}
	for certName, cert := range Config.Certificates {
		if cert.IsSelfSigned {
			if cert.Issuer != "" {
				return fmt.Errorf("self-signed certificate %s must not have issuer", certName)
			}
		} else {
			issuerCert, ok := Config.Certificates[cert.Issuer]
			cert.IssuerCert = issuerCert
			if !ok {
				return fmt.Errorf("certificate %s has missing issuer %s", certName, cert.Issuer)
			} else {
				if !issuerCert.IsCA {
					return fmt.Errorf("certificate %s has issuer %s that is not a CA", certName, cert.Issuer)
				}
			}
		}
	}
	return nil
}
