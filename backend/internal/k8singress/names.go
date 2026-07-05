package k8singress

import (
	"crypto/sha256"
	"encoding/hex"
)

// labelID hashes a row id into a surrogate valid anywhere Kubernetes
// restricts charsets — nanoid ids may contain uppercase/'_' or start/end with
// '-'/'_', all illegal in resource names or at label-value edges.
func labelID(rowID string) string {
	sum := sha256.Sum256([]byte(rowID))
	return hex.EncodeToString(sum[:])[:16]
}

// resourceName is the RFC-1123 object name for a domain row; hostnames only
// ever appear in host/dnsNames fields.
func resourceName(domainID string) string {
	return "domain-" + labelID(domainID)
}

// tlsSecretName is the Secret cert-manager writes the issued certificate to.
func tlsSecretName(domainID string) string {
	return resourceName(domainID) + "-tls"
}
