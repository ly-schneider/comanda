package k8singress

import (
	"context"
	"testing"

	"backend/internal/config"
	"backend/internal/generated/ent/tenantdomain"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	cmfake "github.com/cert-manager/cert-manager/pkg/client/clientset/versioned/fake"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

const testNamespace = "comanda"

// recordingStatusStore captures cert_status writes for assertions.
type recordingStatusStore struct {
	writes map[string]tenantdomain.CertStatus
}

func newRecordingStore() *recordingStatusStore {
	return &recordingStatusStore{writes: map[string]tenantdomain.CertStatus{}}
}

func (s *recordingStatusStore) SetCertStatus(_ context.Context, domainID string, status tenantdomain.CertStatus) error {
	s.writes[domainID] = status
	return nil
}

func testConfig() config.IngressConfig {
	return config.IngressConfig{
		Enabled:        true,
		Namespace:      testNamespace,
		ClassName:      "traefik",
		HTTP01Issuer:   "letsencrypt-http01",
		WebService:     "comanda-web",
		WebServicePort: 80,
	}
}

func newTestReconciler(store statusStore, seed ...*cmapi.Certificate) (*reconciler, *fake.Clientset, *cmfake.Clientset) {
	k8s := fake.NewClientset()
	objs := make([]runtime.Object, len(seed))
	for i, c := range seed {
		objs[i] = c
	}
	cm := cmfake.NewClientset(objs...)
	r := &reconciler{
		status: store,
		cfg:    testConfig(),
		k8s:    k8s,
		cm:     cm,
		logger: zap.NewNop(),
	}
	return r, k8s, cm
}

func TestReconcileAppliesIngressAndCertificate(t *testing.T) {
	store := newRecordingStore()
	r, k8s, cm := newTestReconciler(store)

	d := domain{id: "AB_cd-EF_gh1", tenantID: "tenant______1", hostname: "shop.example.com"}
	if err := r.reconcileDomains(context.Background(), []domain{d}); err != nil {
		t.Fatalf("reconcileDomains: %v", err)
	}

	name := resourceName(d.id)

	ing, err := k8s.NetworkingV1().Ingresses(testNamespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("ingress not created: %v", err)
	}
	if ing.Labels[labelManagedBy] != labelManagedByValue {
		t.Errorf("ingress missing managed-by label: %v", ing.Labels)
	}
	if ing.Labels[labelTenantDomainID] != d.id || ing.Labels[labelTenantID] != d.tenantID {
		t.Errorf("ingress owner labels wrong: %v", ing.Labels)
	}
	if ing.Annotations[annoArgoSyncOptions] != annoArgoSyncValue || ing.Annotations[annoArgoCompareOptions] != annoArgoCompareValue {
		t.Errorf("ingress missing argo annotations: %v", ing.Annotations)
	}
	if ing.Spec.IngressClassName == nil || *ing.Spec.IngressClassName != "traefik" {
		t.Errorf("ingress class wrong: %v", ing.Spec.IngressClassName)
	}
	if len(ing.Spec.TLS) != 1 || ing.Spec.TLS[0].SecretName != tlsSecretName(d.id) {
		t.Errorf("ingress TLS wrong: %v", ing.Spec.TLS)
	}
	if len(ing.Spec.Rules) != 1 || ing.Spec.Rules[0].Host != d.hostname {
		t.Errorf("ingress rule host wrong: %v", ing.Spec.Rules)
	}
	// The Ingress must NOT carry a cert-manager issuer annotation (the explicit
	// Certificate is the single issuance owner).
	for k := range ing.Annotations {
		if k == "cert-manager.io/cluster-issuer" || k == "cert-manager.io/issuer" {
			t.Errorf("ingress unexpectedly carries cert-manager annotation %q", k)
		}
	}

	cert, err := cm.CertmanagerV1().Certificates(testNamespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("certificate not created: %v", err)
	}
	if cert.Spec.SecretName != tlsSecretName(d.id) {
		t.Errorf("certificate secret name wrong: %v", cert.Spec.SecretName)
	}
	if len(cert.Spec.DNSNames) != 1 || cert.Spec.DNSNames[0] != d.hostname {
		t.Errorf("certificate dnsNames wrong: %v", cert.Spec.DNSNames)
	}
	if cert.Spec.IssuerRef.Name != "letsencrypt-http01" || cert.Spec.IssuerRef.Kind != "ClusterIssuer" {
		t.Errorf("certificate issuerRef wrong: %+v", cert.Spec.IssuerRef)
	}
	if cert.Labels[labelManagedBy] != labelManagedByValue {
		t.Errorf("certificate missing managed-by label: %v", cert.Labels)
	}
	if cert.Annotations[annoArgoSyncOptions] != annoArgoSyncValue {
		t.Errorf("certificate missing argo annotation: %v", cert.Annotations)
	}

	// No status on the freshly created cert -> pending.
	if got := store.writes[d.id]; got != tenantdomain.CertStatusPending {
		t.Errorf("expected pending status write, got %q", got)
	}
}

func TestReconcileGarbageCollectsOrphans(t *testing.T) {
	store := newRecordingStore()
	r, k8s, cm := newTestReconciler(store)
	ctx := context.Background()

	// Seed two domains.
	live := domain{id: "live________1", tenantID: "tenant______1", hostname: "live.example.com"}
	orphan := domain{id: "orphan______1", tenantID: "tenant______1", hostname: "orphan.example.com"}
	if err := r.reconcileDomains(ctx, []domain{live, orphan}); err != nil {
		t.Fatalf("seed reconcile: %v", err)
	}

	// Now only `live` remains verified; orphan's objects must be collected.
	if err := r.reconcileDomains(ctx, []domain{live}); err != nil {
		t.Fatalf("gc reconcile: %v", err)
	}

	if _, err := k8s.NetworkingV1().Ingresses(testNamespace).Get(ctx, resourceName(orphan.id), metav1.GetOptions{}); err == nil {
		t.Errorf("orphan ingress was not collected")
	}
	if _, err := cm.CertmanagerV1().Certificates(testNamespace).Get(ctx, resourceName(orphan.id), metav1.GetOptions{}); err == nil {
		t.Errorf("orphan certificate was not collected")
	}
	if _, err := k8s.NetworkingV1().Ingresses(testNamespace).Get(ctx, resourceName(live.id), metav1.GetOptions{}); err != nil {
		t.Errorf("live ingress was wrongly collected: %v", err)
	}
	if _, err := cm.CertmanagerV1().Certificates(testNamespace).Get(ctx, resourceName(live.id), metav1.GetOptions{}); err != nil {
		t.Errorf("live certificate was wrongly collected: %v", err)
	}
}

func TestReconcileStatusFeedback(t *testing.T) {
	d := domain{id: "status______1", tenantID: "tenant______1", hostname: "ready.example.com", certStatus: tenantdomain.CertStatusPending}
	name := resourceName(d.id)

	readyCert := &cmapi.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
			Labels:    ownerLabels(d.id, d.tenantID),
		},
		Status: cmapi.CertificateStatus{
			Conditions: []cmapi.CertificateCondition{
				{Type: cmapi.CertificateConditionReady, Status: cmmeta.ConditionTrue},
			},
		},
	}

	store := newRecordingStore()
	r, _, _ := newTestReconciler(store, readyCert)

	if err := r.reconcileDomains(context.Background(), []domain{d}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if got := store.writes[d.id]; got != tenantdomain.CertStatusActive {
		t.Errorf("expected active status write, got %q", got)
	}
}

func TestReconcileStatusNoWriteWhenUnchanged(t *testing.T) {
	// Domain already active; a ready cert must NOT trigger a redundant write.
	d := domain{id: "noop________1", tenantID: "tenant______1", hostname: "active.example.com", certStatus: tenantdomain.CertStatusActive}
	name := resourceName(d.id)
	readyCert := &cmapi.Certificate{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace, Labels: ownerLabels(d.id, d.tenantID)},
		Status: cmapi.CertificateStatus{
			Conditions: []cmapi.CertificateCondition{{Type: cmapi.CertificateConditionReady, Status: cmmeta.ConditionTrue}},
		},
	}
	store := newRecordingStore()
	r, _, _ := newTestReconciler(store, readyCert)

	if err := r.reconcileDomains(context.Background(), []domain{d}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if _, written := store.writes[d.id]; written {
		t.Errorf("expected no status write when unchanged, got %q", store.writes[d.id])
	}
}

func TestMapCertStatus(t *testing.T) {
	cond := func(status cmmeta.ConditionStatus, reason string) *cmapi.Certificate {
		return &cmapi.Certificate{Status: cmapi.CertificateStatus{
			Conditions: []cmapi.CertificateCondition{
				{Type: cmapi.CertificateConditionReady, Status: status, Reason: reason},
			},
		}}
	}

	cases := []struct {
		name string
		cert *cmapi.Certificate
		want tenantdomain.CertStatus
	}{
		{"ready true -> active", cond(cmmeta.ConditionTrue, "Ready"), tenantdomain.CertStatusActive},
		{"false issuing -> pending", cond(cmmeta.ConditionFalse, "Issuing"), tenantdomain.CertStatusPending},
		{"false failed -> failed", cond(cmmeta.ConditionFalse, "Failed"), tenantdomain.CertStatusFailed},
		{"false too many -> failed", cond(cmmeta.ConditionFalse, "TooManyValidationFailures"), tenantdomain.CertStatusFailed},
		{"no conditions -> pending", &cmapi.Certificate{}, tenantdomain.CertStatusPending},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mapCertStatus(tc.cert); got != tc.want {
				t.Errorf("mapCertStatus = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDisabledReconcilerIsNoop(t *testing.T) {
	cfg := config.IngressConfig{Enabled: false}
	rec, err := New(nil, cfg, zap.NewNop())
	if err != nil {
		t.Fatalf("New(disabled): %v", err)
	}
	if _, ok := rec.(noopReconciler); !ok {
		t.Fatalf("expected noopReconciler, got %T", rec)
	}
	if err := rec.Reconcile(context.Background()); err != nil {
		t.Errorf("noop Reconcile returned error: %v", err)
	}
}
