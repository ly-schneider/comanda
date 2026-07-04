// Package k8singress reconciles Kubernetes Ingress and cert-manager Certificate
// objects for verified custom domains. It is deliberately self-contained: it
// takes the privileged Ent client and config directly and never imports the
// repository or service layers, so it cannot form an import cycle with them.
//
// Resource and Secret names are derived from the domain row id (hashed), never
// from the hostname (the nanoid alphabet and arbitrary hostnames are not
// RFC-1123 safe). Every object is server-side-applied with a fixed field manager
// and carries ArgoCD opt-out annotations so the GitOps controller leaves them
// alone. All operations are pinned to a single namespace.
package k8singress

import (
	"context"
	"fmt"

	"backend/internal/config"
	"backend/internal/generated/ent"
	"backend/internal/generated/ent/tenantdomain"

	cmclient "github.com/cert-manager/cert-manager/pkg/client/clientset/versioned"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	fieldManager = "comanda-ingress-reconciler"

	labelManagedBy      = "app.kubernetes.io/managed-by"
	labelManagedByValue = "comanda-ingress-reconciler"
	labelTenantDomainID = "comanda.leys.ch/tenant-domain-id"
	labelTenantID       = "comanda.leys.ch/tenant-id"

	annoArgoSyncOptions    = "argocd.argoproj.io/sync-options"
	annoArgoSyncValue      = "Prune=false"
	annoArgoCompareOptions = "argocd.argoproj.io/compare-options"
	annoArgoCompareValue   = "IgnoreExtraneous"
	annoTenantDomainID     = "comanda.leys.ch/tenant-domain-id"
	annoTenantID           = "comanda.leys.ch/tenant-id"
)

// Reconciler converges cluster state for verified custom domains.
type Reconciler interface {
	Reconcile(ctx context.Context) error
}

// noopReconciler is used when the reconciler is disabled. It never touches a
// cluster.
type noopReconciler struct{}

func (noopReconciler) Reconcile(context.Context) error { return nil }

// reconciler is the live implementation.
type reconciler struct {
	priv   *ent.Client
	status statusStore
	cfg    config.IngressConfig
	k8s    kubernetes.Interface
	cm     cmclient.Interface
	logger *zap.Logger
}

// entStatusStore writes cert_status transitions through the privileged Ent
// client.
type entStatusStore struct {
	priv *ent.Client
}

func (s entStatusStore) SetCertStatus(ctx context.Context, domainID string, status tenantdomain.CertStatus) error {
	return s.priv.TenantDomain.UpdateOneID(domainID).SetCertStatus(status).Exec(ctx)
}

// New builds a Reconciler. When cfg.Enabled is false it returns a no-op. When
// enabled it builds a rest.Config (KUBECONFIG if set, else in-cluster) and fails
// fast if neither is available — a misconfigured-but-enabled reconciler must not
// silently do nothing.
func New(priv *ent.Client, cfg config.IngressConfig, logger *zap.Logger) (Reconciler, error) {
	if !cfg.Enabled {
		return noopReconciler{}, nil
	}
	if cfg.Namespace == "" {
		return nil, fmt.Errorf("k8singress: INGRESS_NAMESPACE is required when the reconciler is enabled")
	}
	if cfg.HTTP01Issuer == "" {
		return nil, fmt.Errorf("k8singress: INGRESS_HTTP01_ISSUER is required when the reconciler is enabled")
	}
	if cfg.ManagedZone != "" && cfg.DNS01Issuer == "" {
		return nil, fmt.Errorf("k8singress: INGRESS_DNS01_ISSUER is required when INGRESS_MANAGED_ZONE is set")
	}

	restCfg, err := restConfig(cfg.Kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("k8singress: build rest config: %w", err)
	}

	k8s, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("k8singress: build kubernetes client: %w", err)
	}
	cm, err := cmclient.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("k8singress: build cert-manager client: %w", err)
	}

	return &reconciler{
		priv:   priv,
		status: entStatusStore{priv: priv},
		cfg:    cfg,
		k8s:    k8s,
		cm:     cm,
		logger: logger,
	}, nil
}

// restConfig prefers an explicit kubeconfig (dev) and falls back to the
// in-cluster ServiceAccount config.
func restConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}

// ownerLabels scope GC and map objects back to their domain row. Values are
// hashed (labelID) — a nanoid ending in '-'/'_' fails label validation;
// the raw ids live in annotations.
func ownerLabels(domainID, tenantID string) map[string]string {
	return map[string]string{
		labelManagedBy:      labelManagedByValue,
		labelTenantDomainID: labelID(domainID),
		labelTenantID:       labelID(tenantID),
	}
}

// ownerAnnotations opt managed objects out of ArgoCD's prune + self-heal and
// carry the raw row ids (annotation values are charset-unrestricted).
func ownerAnnotations(domainID, tenantID string) map[string]string {
	return map[string]string{
		annoArgoSyncOptions:    annoArgoSyncValue,
		annoArgoCompareOptions: annoArgoCompareValue,
		annoTenantDomainID:     domainID,
		annoTenantID:           tenantID,
	}
}
