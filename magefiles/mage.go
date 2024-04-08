//go:build mage

package main

import (
	goutil "github.com/elisasre/mageutil/golang"

	//mage:import
	_ "github.com/elisasre/mageutil/cyclonedx/target"
	//mage:import
	_ "github.com/elisasre/mageutil/golangcilint/target"
	//mage:import
	golang "github.com/elisasre/mageutil/golang/target"
)

// Configure imported targets
func init() {
	golang.BuildTarget = "./cmd/resource-advisor"
	golang.BuildMatrix = append(goutil.DefaultBuildMatrix, goutil.BuildPlatform{OS: "windows", Arch: "amd64"})
}
