/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"

	warpgatev1alpha1 "github.com/thereisnotime/warpgate-operator/api/v1alpha1"
)

func seedTestInstance(version, image string) *warpgatev1alpha1.WarpgateInstance {
	inst := &warpgatev1alpha1.WarpgateInstance{}
	inst.Name = "wg"
	inst.Namespace = "default"
	inst.Spec.Version = version
	inst.Spec.Image = image
	inst.Spec.AdminPasswordSecretRef = warpgatev1alpha1.SecretKeyRef{Name: "admin-pw"}
	return inst
}

func TestImageHasShell(t *testing.T) {
	cases := []struct {
		version, image string
		wantShell      bool
	}{
		{version: "0.21.1", wantShell: true},
		{version: "v0.21.1", wantShell: true},
		{version: "0.26.0", wantShell: true},
		{version: "0.26.5", wantShell: true},
		{version: "0.27.0", wantShell: false}, // first distroless release
		{version: "0.30.6", wantShell: false},
		{version: "latest", wantShell: false}, // unparseable -> assume shell-less
		{version: "", wantShell: false},       // defaults to 0.30.6
		{image: "ghcr.io/warp-tech/warpgate:0.26.0", wantShell: true},
		{image: "ghcr.io/warp-tech/warpgate:0.30.6", wantShell: false},
		{image: "registry.example.com:5000/warpgate:0.26.0", wantShell: true}, // registry port must not fool the tag parser
		{image: "myregistry/custom-warpgate@sha256:deadbeef", wantShell: false},
	}
	for _, c := range cases {
		inst := seedTestInstance(c.version, c.image)
		if got := imageHasShell(inst); got != c.wantShell {
			t.Errorf("imageHasShell(version=%q image=%q) = %v, want %v", c.version, c.image, got, c.wantShell)
		}
	}
}

func TestResolveImageDefaultsVersion(t *testing.T) {
	inst := seedTestInstance("", "")
	want := "ghcr.io/warp-tech/warpgate:" + warpgatev1alpha1.DefaultWarpgateVersion
	if got := resolveImage(inst); got != want {
		t.Errorf("resolveImage with no version/image = %q, want %q", got, want)
	}
	if warpgatev1alpha1.DefaultWarpgateVersion != "0.30.6" {
		t.Errorf("DefaultWarpgateVersion = %q, want 0.30.6", warpgatev1alpha1.DefaultWarpgateVersion)
	}
}

func TestSeedImageOverride(t *testing.T) {
	inst := seedTestInstance("0.30.6", "")
	if got := seedImage(inst); got != warpgatev1alpha1.DefaultSeedImage {
		t.Errorf("seedImage default = %q, want %q", got, warpgatev1alpha1.DefaultSeedImage)
	}
	if !strings.Contains(warpgatev1alpha1.DefaultSeedImage, "@sha256:") {
		t.Errorf("DefaultSeedImage must be pinned by digest, got %q", warpgatev1alpha1.DefaultSeedImage)
	}
	inst.Spec.SeedImage = "busybox:custom"
	if got := seedImage(inst); got != "busybox:custom" {
		t.Errorf("seedImage override = %q, want busybox:custom", got)
	}
}

func TestWarpgateSetupArgsShellFree(t *testing.T) {
	inst := seedTestInstance("0.30.6", "")
	args := warpgateSetupArgs(inst)
	joined := strings.Join(args, " ")

	for _, want := range []string{
		"--config /data/warpgate.yaml",
		"--skip-securing-files",
		"unattended-setup",
		"--if-config-exists skip", // replaces the shell "[ -f ]" guard
		"--admin-password $(ADMIN_PASSWORD)",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("setup args missing %q; got: %s", want, joined)
		}
	}
	// The setup container is the Warpgate image's entrypoint (the warpgate binary),
	// so it must never invoke a shell.
	for _, a := range args {
		if strings.Contains(a, "/bin/sh") || strings.Contains(a, "/bin/bash") {
			t.Errorf("setup args must not reference a shell, got: %v", args)
		}
	}
}

// TestBuildInitContainersDistroless asserts the config seeding for a distroless
// (>= 0.27) Warpgate image does not depend on a shell in the Warpgate image.
func TestBuildInitContainersDistroless(t *testing.T) {
	inst := seedTestInstance("0.30.6", "")
	image := resolveImage(inst)
	script := "ignored-shell-script"
	containers, material := buildInitContainers(inst, image, script,
		[]corev1.EnvVar{{Name: "ADMIN_PASSWORD"}}, []corev1.VolumeMount{{Name: "data"}})

	if len(containers) != 2 {
		t.Fatalf("distroless path should produce 2 init containers, got %d", len(containers))
	}

	setup := containers[0]
	if setup.Image != image {
		t.Errorf("setup container image = %q, want Warpgate image %q", setup.Image, image)
	}
	if len(setup.Command) != 0 {
		t.Errorf("setup container must not override Command with a shell, got %v", setup.Command)
	}
	if strings.Join(setup.Args, " ") == "" || !strings.Contains(strings.Join(setup.Args, " "), "unattended-setup") {
		t.Errorf("setup container must run unattended-setup via Args, got %v", setup.Args)
	}

	cfg := containers[1]
	if cfg.Image == image {
		t.Errorf("config-seed container must NOT use the Warpgate image (it has no shell); got %q", cfg.Image)
	}
	if !strings.Contains(cfg.Image, "busybox") {
		t.Errorf("config-seed container should default to busybox, got %q", cfg.Image)
	}
	if len(cfg.Command) < 3 || cfg.Command[0] != "/bin/sh" {
		t.Errorf("config-seed container should run /bin/sh -c <script>, got Command=%v", cfg.Command)
	}
	if !strings.Contains(cfg.Command[2], "cp /config/warpgate.yaml /data/warpgate.yaml") {
		t.Errorf("config-seed script should copy the operator config, got:\n%s", cfg.Command[2])
	}
	// The rollout material must not be the shell script (it is unused on this path).
	if material == script {
		t.Errorf("distroless rollout material should reflect the distroless init steps, not the shell script")
	}
}

// TestBuildInitContainersShellPath asserts a shell-bearing image (<= 0.26) keeps
// the original single in-image init container, so existing 0.26 deployments are
// unchanged.
func TestBuildInitContainersShellPath(t *testing.T) {
	inst := seedTestInstance("0.26.0", "")
	image := resolveImage(inst)
	script := "#!/bin/sh\necho hi"
	containers, material := buildInitContainers(inst, image, script,
		[]corev1.EnvVar{{Name: "ADMIN_PASSWORD"}}, []corev1.VolumeMount{{Name: "data"}})

	if len(containers) != 1 {
		t.Fatalf("shell path should produce 1 init container, got %d", len(containers))
	}
	c := containers[0]
	if c.Image != image {
		t.Errorf("shell init container should use the Warpgate image %q, got %q", image, c.Image)
	}
	if len(c.Command) != 3 || c.Command[0] != "/bin/sh" || c.Command[2] != script {
		t.Errorf("shell init container should run the in-image shell script unchanged, got %v", c.Command)
	}
	if material != script {
		t.Errorf("shell rollout material should be the script, got %q", material)
	}
}

// TestBuildInitContainersNonRootOwnership is the regression test for the
// production bug where the busybox seed container ran as root and wrote a
// root-owned /data/warpgate.yaml the non-root Warpgate user could not read
// ("Could not load config: Permission denied"). Both distroless init containers
// must run as the runtime uid/gid, and the seed script must leave files group
// readable.
func TestBuildInitContainersNonRootOwnership(t *testing.T) {
	inst := seedTestInstance("0.30.6", "")
	containers, _ := buildInitContainers(inst, resolveImage(inst), "unused",
		[]corev1.EnvVar{{Name: "ADMIN_PASSWORD"}}, []corev1.VolumeMount{{Name: "data"}})

	if len(containers) != 2 {
		t.Fatalf("distroless path should produce 2 init containers, got %d", len(containers))
	}
	for _, c := range containers {
		sc := c.SecurityContext
		if sc == nil || sc.RunAsUser == nil || sc.RunAsGroup == nil {
			t.Fatalf("init container %q must set RunAsUser/RunAsGroup so seeded files are owned by the runtime user", c.Name)
		}
		if *sc.RunAsUser != 1000 || *sc.RunAsGroup != 1000 {
			t.Errorf("init container %q RunAsUser/Group = %d/%d, want 1000/1000 (the Warpgate image uid/gid)", c.Name, *sc.RunAsUser, *sc.RunAsGroup)
		}
		if sc.RunAsNonRoot == nil || !*sc.RunAsNonRoot {
			t.Errorf("init container %q must assert RunAsNonRoot so a restricted PodSecurity policy does not fall back to root", c.Name)
		}
	}
}

func TestSeedSecurityContextRespectsPodRunAsUser(t *testing.T) {
	inst := seedTestInstance("0.30.6", "")
	uid := int64(2000)
	gid := int64(3000)
	inst.Spec.PodSecurityContext = &corev1.PodSecurityContext{RunAsUser: &uid, RunAsGroup: &gid}

	sc := seedSecurityContext(inst)
	// The seed must match whatever user the main container runs as, so a pinned
	// pod RunAsUser propagates to the init containers.
	if sc.RunAsUser == nil || *sc.RunAsUser != 2000 || sc.RunAsGroup == nil || *sc.RunAsGroup != 3000 {
		t.Errorf("seedSecurityContext should follow pod RunAsUser/Group 2000/3000, got %v/%v", sc.RunAsUser, sc.RunAsGroup)
	}
}

func TestSeedScriptGroupReadable(t *testing.T) {
	inst := seedTestInstance("0.30.6", "")
	inst.Spec.TLS = &warpgatev1alpha1.InstanceTLSSpec{SecretName: "wg-tls"}
	script := buildSeedScript(inst)

	// The config and any seeded PEM must be group readable (0640), never
	// owner-only (0600), so the runtime user can read them via the fsGroup.
	if strings.Contains(script, "chmod 600 ") || strings.Contains(script, "chmod 0600 ") {
		t.Errorf("seed script must not chmod files to owner-only; got:\n%s", script)
	}
	if !strings.Contains(script, "chmod 0640 /data/warpgate.yaml") {
		t.Errorf("seed script should chmod the config to 0640; got:\n%s", script)
	}
	if !strings.Contains(script, "chmod 0640 /data/tls.certificate.pem /data/tls.key.pem") {
		t.Errorf("seed script should chmod the copied TLS pair to 0640; got:\n%s", script)
	}
}

// TestSeedScriptSelfHealsPoisonedConfig is the regression test for the bricked
// PVC: an older seed that ran as root left /data/warpgate.yaml root-owned, which
// a non-root `cp` cannot overwrite. The seed must unlink the file before copying
// so the volume recovers automatically. Covers both seed paths.
func TestSeedScriptSelfHealsPoisonedConfig(t *testing.T) {
	// Distroless (>= 0.27) seed script.
	distroless := buildSeedScript(seedTestInstance("0.30.6", ""))
	assertRemoveBeforeCopy(t, "distroless", distroless)

	// Shell (<= 0.26) seed script, taken from the real built Deployment.
	inst := seedTestInstance("0.26.0", "")
	deploy := (&WarpgateInstanceReconciler{}).buildDeployment(inst)
	ics := deploy.Spec.Template.Spec.InitContainers
	if len(ics) != 1 || len(ics[0].Command) < 3 {
		t.Fatalf("shell path should be a single /bin/sh -c init container, got %d", len(ics))
	}
	assertRemoveBeforeCopy(t, "shell", ics[0].Command[2])
}

// assertRemoveBeforeCopy checks the script removes /data/warpgate.yaml before it
// copies a new one into place (self-heal), not after.
func assertRemoveBeforeCopy(t *testing.T, label, script string) {
	t.Helper()
	rm := strings.Index(script, "rm -f /data/warpgate.yaml")
	if rm < 0 {
		t.Errorf("%s seed must `rm -f /data/warpgate.yaml` to self-heal a root-owned file; got:\n%s", label, script)
		return
	}
	cpCfg := indexOfCopyToConfig(script)
	if cpCfg < 0 {
		t.Errorf("%s seed must copy a config into /data/warpgate.yaml; got:\n%s", label, script)
		return
	}
	if rm > cpCfg {
		t.Errorf("%s seed must remove the old config BEFORE copying the new one (rm@%d, cp@%d)", label, rm, cpCfg)
	}
}

func indexOfCopyToConfig(script string) int {
	for _, needle := range []string{
		"cp /config/warpgate.yaml /data/warpgate.yaml",
		"cp /override/warpgate.yaml /data/warpgate.yaml",
	} {
		if i := strings.Index(script, needle); i >= 0 {
			return i
		}
	}
	return -1
}

func TestResolveImageStripsLeadingV(t *testing.T) {
	// Upstream ghcr tags have no leading v: v0.30.6 must normalise to 0.30.6.
	cases := map[string]string{
		"0.30.6":  "ghcr.io/warp-tech/warpgate:0.30.6",
		"v0.30.6": "ghcr.io/warp-tech/warpgate:0.30.6",
		"0.26.0":  "ghcr.io/warp-tech/warpgate:0.26.0",
	}
	for version, want := range cases {
		if got := resolveImage(seedTestInstance(version, "")); got != want {
			t.Errorf("resolveImage(%q) = %q, want %q", version, got, want)
		}
	}
}

// TestExternalPortDefaults443 covers the SSO redirect_uri regression: an
// instance with externalHost set must advertise external_port 443, not the
// in-cluster listener port, so the OIDC redirect_uri carries no :8888.
func TestExternalPortDefaults443(t *testing.T) {
	inst := seedTestInstance("0.30.6", "")
	inst.Spec.ExternalHost = "warpgate.example.com"

	port, ok := instanceHTTPExternalPort(inst)
	if !ok || port != 443 {
		t.Fatalf("instanceHTTPExternalPort with externalHost set = (%d,%v), want (443,true)", port, ok)
	}
	cfg := (&WarpgateInstanceReconciler{}).buildWarpgateConfig(inst)
	if !strings.Contains(cfg, "external_port: 443") {
		t.Errorf("config should set external_port: 443; got:\n%s", cfg)
	}
}

// TestSSOProviderRendersSentinelNotSecret covers item 8: an SSO provider renders
// into warpgate.yaml with a sentinel in place of the client secret, which the
// init script later swaps for the real value from the environment.
func TestSSOProviderRendersSentinelNotSecret(t *testing.T) {
	inst := seedTestInstance("0.30.6", "")
	inst.Spec.SSO = &warpgatev1alpha1.SSOSpec{
		Providers: []warpgatev1alpha1.SSOProviderSpec{{
			Name:                  "authentik",
			Label:                 "Authentik",
			ClientID:              "warpgate-client",
			IssuerURL:             "https://sso.example.com/application/o/warpgate/",
			ClientSecretSecretRef: warpgatev1alpha1.SecretKeyRef{Name: "sso-secret", Key: "client-secret"},
		}},
	}
	cfg := (&WarpgateInstanceReconciler{}).buildWarpgateConfig(inst)
	for _, want := range []string{"warpgate-client", "https://sso.example.com/application/o/warpgate/", ssoClientSecretSentinel("authentik")} {
		if !strings.Contains(cfg, want) {
			t.Errorf("SSO config missing %q; got:\n%s", want, cfg)
		}
	}
}

// TestBuildSeedScriptNoSetupNoOpenssl asserts the busybox seed script does not
// run unattended-setup (the setup container owns that) or openssl (Warpgate's
// setup generates the self-signed cert), so busybox is sufficient.
func TestBuildSeedScriptNoSetupNoOpenssl(t *testing.T) {
	inst := seedTestInstance("0.30.6", "")
	script := buildSeedScript(inst)
	for _, banned := range []string{"unattended-setup", "openssl", "apk add", "warpgate "} {
		if strings.Contains(script, banned) {
			t.Errorf("seed script should not contain %q; got:\n%s", banned, script)
		}
	}
	if !strings.Contains(script, "cp /config/warpgate.yaml /data/warpgate.yaml") {
		t.Errorf("seed script should apply the operator config; got:\n%s", script)
	}
}
