// Copyright 2021 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package bundle

import (
	"bytes"
	"fmt"
	ctlconf "github.com/vmware-tanzu/carvel-vendir/pkg/vendir/config"
	ctlfetch "github.com/vmware-tanzu/carvel-vendir/pkg/vendir/fetch"
	"os/exec"
	"regexp"
	"strings"
)

type Sync struct {
	opts       ctlconf.DirectoryContentsBundle
	refFetcher ctlfetch.RefFetcher
}

func NewSync(opts ctlconf.DirectoryContentsBundle, refFetcher ctlfetch.RefFetcher) *Sync {
	return &Sync{opts, refFetcher}
}

var (
	// Example image ref in imgpkg stdout:
	//   Pulling image 'index.docker.io/dkalinin/consul-helm@sha256:d1cdbd46561a144332f0744302d45f27583fc0d75002cba473d840f46630c9f7'
	imgpkgPulledImageRef = regexp.MustCompile("(?m)^Pulling bundle '(.+)'$")
)

func (t *Sync) Sync(dstPath string) (ctlconf.LockDirectoryContentsBundle, error) {
	lockConf := ctlconf.LockDirectoryContentsBundle{}

	if len(t.opts.URL) == 0 {
		return lockConf, fmt.Errorf("Expected non-empty URL")
	}

	args := []string{"pull", "-b", t.opts.URL, "-o", dstPath, "--tty=true"}

	args, err := t.addAuthArgs(args)
	if err != nil {
		return lockConf, err
	}

	var stdoutBs, stderrBs bytes.Buffer

	cmd := exec.Command("imgpkg", args...)
	cmd.Stdout = &stdoutBs
	cmd.Stderr = &stderrBs

	err = cmd.Run()
	if err != nil {
		return lockConf, fmt.Errorf("Imgpkg: %s (stderr: %s)", err, stderrBs.String())
	}

	stdoutStr := stdoutBs.String()

	matches := imgpkgPulledImageRef.FindStringSubmatch(stdoutStr)
	if len(matches) != 2 {
		return lockConf, fmt.Errorf("Expected to find pulled image ref in stdout, but did not (stdout: '%s')", stdoutStr)
	}
	if !strings.Contains(matches[1], "@") {
		return lockConf, fmt.Errorf("Expected ref '%s' to be in digest form, but was not", matches[1])
	}

	lockConf.URL = matches[1]

	return lockConf, nil
}

func (t *Sync) addAuthArgs(args []string) ([]string, error) {
	var authArgs []string

	if t.opts.SecretRef != nil {
		secret, err := t.refFetcher.GetSecret(t.opts.SecretRef.Name)
		if err != nil {
			return nil, err
		}

		for name, val := range secret.Data {
			switch name {
			case ctlconf.SecretK8sCorev1BasicAuthUsernameKey:
				authArgs = append(authArgs, []string{"--registry-username", string(val)}...)
			case ctlconf.SecretK8sCorev1BasicAuthPasswordKey:
				authArgs = append(authArgs, []string{"--registry-password", string(val)}...)
			case ctlconf.SecretToken:
				authArgs = append(authArgs, []string{"--registry-token", string(val)}...)
			default:
				return nil, fmt.Errorf("Unknown secret field '%s' in secret '%s'", name, secret.Metadata.Name)
			}
		}
	}

	if len(authArgs) == 0 {
		authArgs = []string{"--registry-anon"}
	}

	return append(args, authArgs...), nil
}
