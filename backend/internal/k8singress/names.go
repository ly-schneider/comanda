package k8singress

import (
	"crypto/sha256"
	"encoding/hex"
)

// resourceName derives an RFC-1123-compliant Kubernetes object name from a
// domain row id. The nanoid alphabet contains uppercase letters and '_', both
// illegal in K8s resource names, so the id is hashed rather than used directly.
// Only host/dnsNames ever carry the actual hostname; resource/secret names are
// always id-derived to keep them deterministic and DNS-safe.
func resourceName(domainID string) string {
	sum := sha256.Sum256([]byte(domainID))
	return "domain-" + hex.EncodeToString(sum[:])[:16]
}

// tlsSecretName is the Secret cert-manager writes the issued certificate to.
func tlsSecretName(domainID string) string {
	return resourceName(domainID) + "-tls"
}
