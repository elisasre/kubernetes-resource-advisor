//go:build mage

package main

import (
	"context"
	"os"

	"github.com/elisasre/mageutil"
)

const (
	AppName   = "resource-advisor"
	RepoURL   = "https://github.com/elisasre/ingress-watcher"
	ImageName = "quay.io/elisaoyj/ingress-watcher"
)

// Build for executables under ./cmd
func Build(ctx context.Context) error {
	return mageutil.BuildAll(ctx)
}

// Build all platform binaries for executables under ./cmd
func BuildSubset(ctx context.Context) {
	mageutil.BuildForLinux(ctx, AppName)
	mageutil.BuildForMac(ctx, AppName)
	mageutil.BuildForArmMac(ctx, AppName)
	mageutil.BuildForWindows(ctx, AppName)
}

// UnitTest whole repo
func UnitTest(ctx context.Context) error {
	return mageutil.UnitTest(ctx)
}

// IntegrationTest whole repo
func IntegrationTest(ctx context.Context) error {
	return mageutil.IntegrationTest(ctx, "./cmd/"+AppName)
}

// Lint all go files.
func Lint(ctx context.Context) error {
	return mageutil.LintAll(ctx)
}

// VulnCheck all go files.
func VulnCheck(ctx context.Context) error {
	return mageutil.VulnCheckAll(ctx)
}

// LicenseCheck all files.
func LicenseCheck(ctx context.Context) error {
	return mageutil.LicenseCheck(ctx, os.Stdout, mageutil.CmdDir+AppName)
}

// Clean removes all files ignored by git
func Clean(ctx context.Context) error {
	return mageutil.Clean(ctx)
}
