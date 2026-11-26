package controller

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	warpgatev1alpha1 "github.com/thereisnotime/warpgate-operator/api/v1alpha1"
)

func inst(externalHost string, tls *warpgatev1alpha1.InstanceTLSSpec) *warpgatev1alpha1.WarpgateInstance {
	return &warpgatev1alpha1.WarpgateInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "hz", Namespace: "warpgate"},
		Spec: warpgatev1alpha1.WarpgateInstanceSpec{
			ExternalHost: externalHost,
			TLS:          tls,
		},
	}
}

func TestCertificateDNSNames(t *testing.T) {
	issuer := &warpgatev1alpha1.InstanceTLSSpec{
		IssuerRef: &warpgatev1alpha1.CertIssuerRef{Name: "letsencrypt-prod"},
	}
	cases := []struct {
		name string
		in   *warpgatev1alpha1.WarpgateInstance
		want []string
	}{
		{
			name: "self-signed (no issuer) keeps internal SANs + external",
			in:   inst("gate.hz.rso.dev", nil),
			want: []string{"hz-http.warpgate.svc", "hz-http.warpgate.svc.cluster.local", "gate.hz.rso.dev"},
		},
		{
			name: "public issuer: only the public name, no internal SANs",
			in:   inst("gate.hz.rso.dev", issuer),
			want: []string{"gate.hz.rso.dev"},
		},
		{
			name: "public issuer, no externalHost: empty (skip cert)",
			in:   inst("", issuer),
			want: nil,
		},
		{
			name: "self-signed, no externalHost: internal SANs only",
			in:   inst("", nil),
			want: []string{"hz-http.warpgate.svc", "hz-http.warpgate.svc.cluster.local"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := certificateDNSNames(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}
