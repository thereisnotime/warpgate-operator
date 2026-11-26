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
