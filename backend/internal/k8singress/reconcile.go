package k8singress

import (
	"context"
	"fmt"

	"backend/internal/dnsname"
	"backend/internal/generated/ent/tenantdomain"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	cmac "github.com/cert-manager/cert-manager/pkg/client/applyconfigurations/certmanager/v1"
	cmmetaac "github.com/cert-manager/cert-manager/pkg/client/applyconfigurations/meta/v1"
	"go.uber.org/zap"
	netv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	netv1ac "k8s.io/client-go/applyconfigurations/networking/v1"
)

const managedBySelector = labelManagedBy + "=" + labelManagedByValue

// domain is the reconciler's view of a verified custom domain, decoupled from
// the Ent row so the K8s logic is testable without a database.
type domain struct {
	id         string
	tenantID   string
	hostname   string
	certStatus tenantdomain.CertStatus
}

// statusStore persists cert_status transitions. The live reconciler backs it
// with the privileged Ent client; tests stub it.
type statusStore interface {
	SetCertStatus(ctx context.Context, domainID string, status tenantdomain.CertStatus) error
}

// Reconcile converges Ingress + Certificate objects for every verified custom
// domain, feeds cert status back into the database, and garbage-collects objects
// whose domain is no longer verified.
func (r *reconciler) Reconcile(ctx context.Context) error {
	rows, err := r.priv.TenantDomain.Query().
		Where(tenantdomain.VerifiedAtNotNil()).
		All(ctx)
	if err != nil {
		return fmt.Errorf("k8singress: list verified domains: %w", err)
	}

	domains := make([]domain, 0, len(rows))
	for _, row := range rows {
		domains = append(domains, domain{
			id:         row.ID,
			tenantID:   row.TenantID,
			hostname:   row.Hostname,
			certStatus: row.CertStatus,
		})
	}
	return r.reconcileDomains(ctx, domains)
}

// reconcileDomains is the cluster-facing core: it applies objects, syncs status,
// and garbage-collects. It takes plain domain values and the status store, so it
// runs against fake clientsets in tests with no database.
func (r *reconciler) reconcileDomains(ctx context.Context, domains []domain) error {
	live := make(map[string]struct{}, len(domains))
	for _, d := range domains {
		// Defense-in-depth: the API validates hostnames on create, but a row can
		// reach a verified state by other paths (seed data, manual SQL). Never ask
		// a public CA to issue for a non-public name like "localhost" or an IP —
		// ACME hard-rejects it ("needs at least one dot"), leaving a Certificate
		// stuck un-issued forever (and tripping the platform cert-expiry alert).
		// Leaving the id out of `live` also makes collectGarbage tear down any
		// doomed objects a prior reconcile already created for it.
		if !dnsname.IsPublicFQDN(d.hostname) {
			r.logger.Warn("k8singress: skipping non-issuable hostname; not requesting a certificate",
				zap.String("domainId", d.id),
				zap.String("hostname", d.hostname))
			if err := r.writeCertStatus(ctx, d, tenantdomain.CertStatusFailed); err != nil {
				r.logger.Warn("k8singress: cert status write failed",
					zap.String("domainId", d.id), zap.Error(err))
			}
			continue
		}
		// A hostname the platform's *.{base} wildcard already serves needs no
		// per-domain Certificate: issuing one wastes Let's Encrypt quota, and for a
		// Cloudflare-proxied platform subdomain the HTTP-01 solver can never validate,
		// leaving it stuck. Report it active (the wildcard provides TLS) and leave it
		// out of `live` so collectGarbage tears down any object a prior reconcile made.
		plan := planFor(d.hostname, r.cfg)
		if plan.skip {
			if err := r.writeCertStatus(ctx, d, tenantdomain.CertStatusActive); err != nil {
				r.logger.Warn("k8singress: cert status write failed",
					zap.String("domainId", d.id), zap.Error(err))
			}
			continue
		}

		live[d.id] = struct{}{}
		if err := r.applyDomain(ctx, d, plan.issuer); err != nil {
			r.logger.Warn("k8singress: apply domain failed",
				zap.String("domainId", d.id),
				zap.String("hostname", d.hostname),
				zap.Error(err))
			continue
		}
		if err := r.syncCertStatus(ctx, d); err != nil {
			r.logger.Warn("k8singress: cert status sync failed",
				zap.String("domainId", d.id),
				zap.String("hostname", d.hostname),
				zap.Error(err))
		}
	}

	if err := r.collectGarbage(ctx, live); err != nil {
		r.logger.Warn("k8singress: garbage collection failed", zap.Error(err))
	}
	return nil
}

// applyDomain server-side-applies the Ingress and the explicit Certificate for
// one domain. The Certificate is the single owner of certificate issuance — the
// Ingress deliberately carries no cert-manager issuer annotation so ingress-shim
// never creates a second, competing Certificate.
func (r *reconciler) applyDomain(ctx context.Context, d domain, issuer string) error {
	if err := r.applyIngress(ctx, d); err != nil {
		return fmt.Errorf("apply ingress: %w", err)
	}
	if err := r.applyCertificate(ctx, d, issuer); err != nil {
		return fmt.Errorf("apply certificate: %w", err)
	}
	return nil
}

func (r *reconciler) applyIngress(ctx context.Context, d domain) error {
	name := resourceName(d.id)
	secret := tlsSecretName(d.id)
	pathType := netv1.PathTypePrefix

	ing := netv1ac.Ingress(name, r.cfg.Namespace).
		WithLabels(ownerLabels(d.id, d.tenantID)).
		WithAnnotations(argoAnnotations()).
		WithSpec(netv1ac.IngressSpec().
			WithIngressClassName(r.cfg.ClassName).
			WithTLS(netv1ac.IngressTLS().
				WithHosts(d.hostname).
				WithSecretName(secret)).
			WithRules(netv1ac.IngressRule().
				WithHost(d.hostname).
				WithHTTP(netv1ac.HTTPIngressRuleValue().
					WithPaths(netv1ac.HTTPIngressPath().
						WithPath("/").
						WithPathType(pathType).
						WithBackend(netv1ac.IngressBackend().
							WithService(netv1ac.IngressServiceBackend().
								WithName(r.cfg.WebService).
								WithPort(netv1ac.ServiceBackendPort().
									WithNumber(int32(r.cfg.WebServicePort))))))))) //nolint:gosec // port is bounded config

	_, err := r.k8s.NetworkingV1().Ingresses(r.cfg.Namespace).
		Apply(ctx, ing, metav1.ApplyOptions{FieldManager: fieldManager, Force: true})
	return err
}

func (r *reconciler) applyCertificate(ctx context.Context, d domain, issuer string) error {
	name := resourceName(d.id)
	secret := tlsSecretName(d.id)

	cert := cmac.Certificate(name, r.cfg.Namespace).
		WithLabels(ownerLabels(d.id, d.tenantID)).
		WithAnnotations(argoAnnotations()).
		WithSpec(cmac.CertificateSpec().
			WithSecretName(secret).
			WithDNSNames(d.hostname).
			WithIssuerRef(cmmetaac.IssuerReference().
				WithName(issuer).
				WithKind("ClusterIssuer").
				WithGroup("cert-manager.io")))

	// Re-applying the identical spec is a no-op and never resets cert-manager's
	// Let's Encrypt backoff; a Certificate is NEVER deleted+recreated on failure.
	_, err := r.cm.CertmanagerV1().Certificates(r.cfg.Namespace).
		Apply(ctx, cert, metav1.ApplyOptions{FieldManager: fieldManager, Force: true})
	return err
}

// syncCertStatus reads the Certificate's Ready condition and writes the mapped
// cert_status back to the database, but only when it changed.
func (r *reconciler) syncCertStatus(ctx context.Context, d domain) error {
	name := resourceName(d.id)
	cert, err := r.cm.CertmanagerV1().Certificates(r.cfg.Namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return r.writeCertStatus(ctx, d, tenantdomain.CertStatusPending)
		}
		return err
	}

	newStatus := mapCertStatus(cert)
	return r.writeCertStatus(ctx, d, newStatus)
}

func (r *reconciler) writeCertStatus(ctx context.Context, d domain, status tenantdomain.CertStatus) error {
	if d.certStatus == status {
		return nil
	}
	return r.status.SetCertStatus(ctx, d.id, status)
}

// mapCertStatus maps a Certificate's Ready condition onto the domain cert_status
// enum. Ready True -> active. Ready False with a terminal reason (issuance
// failed / too many attempts) -> failed. Anything else still in flight -> pending.
func mapCertStatus(cert *cmapi.Certificate) tenantdomain.CertStatus {
	for _, cond := range cert.Status.Conditions {
		if cond.Type != cmapi.CertificateConditionReady {
			continue
		}
		switch cond.Status {
		case cmmeta.ConditionTrue:
			return tenantdomain.CertStatusActive
		case cmmeta.ConditionFalse:
			if isTerminalFailure(cond.Reason) {
				return tenantdomain.CertStatusFailed
			}
			return tenantdomain.CertStatusPending
		}
	}
	return tenantdomain.CertStatusPending
}

// isTerminalFailure reports whether a Ready=False reason represents a giving-up
// state rather than an in-progress issuance.
func isTerminalFailure(reason string) bool {
	switch reason {
	case "Failed", "TooManyValidationFailures", "InvalidRequest":
		return true
	default:
		return false
	}
}

// collectGarbage deletes managed Ingresses and Certificates whose domain id is
// no longer in the verified set (domain removed or unverified). The TLS Secret
// is intentionally left to orphan: the reconciler is not granted secrets:delete,
// and an orphaned Secret is harmless.
func (r *reconciler) collectGarbage(ctx context.Context, live map[string]struct{}) error {
	listOpts := metav1.ListOptions{LabelSelector: managedBySelector}

	ingresses, err := r.k8s.NetworkingV1().Ingresses(r.cfg.Namespace).List(ctx, listOpts)
	if err != nil {
		return fmt.Errorf("list ingresses: %w", err)
	}
	for i := range ingresses.Items {
		obj := &ingresses.Items[i]
		if _, ok := live[obj.Labels[labelTenantDomainID]]; ok {
			continue
		}
		if err := r.k8s.NetworkingV1().Ingresses(r.cfg.Namespace).
			Delete(ctx, obj.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			r.logger.Warn("k8singress: delete orphan ingress failed",
				zap.String("name", obj.Name), zap.Error(err))
		}
	}

	certs, err := r.cm.CertmanagerV1().Certificates(r.cfg.Namespace).List(ctx, listOpts)
	if err != nil {
		return fmt.Errorf("list certificates: %w", err)
	}
	for i := range certs.Items {
		obj := &certs.Items[i]
		if _, ok := live[obj.Labels[labelTenantDomainID]]; ok {
			continue
		}
		if err := r.cm.CertmanagerV1().Certificates(r.cfg.Namespace).
			Delete(ctx, obj.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			r.logger.Warn("k8singress: delete orphan certificate failed",
				zap.String("name", obj.Name), zap.Error(err))
		}
	}
	return nil
}
