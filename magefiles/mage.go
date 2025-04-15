//go:build mage

package main

import (
	goutil "github.com/elisasre/mageutil/golang"

	//mage:import
	_ "github.com/elisasre/mageutil/cyclonedx/target"
	//mage:import go
	_ "github.com/elisasre/mageutil/tool/golangcilint"
	//mage:import
	golang "github.com/elisasre/mageutil/golang/target"
)

// Configure imported targets
func init() {
	golang.BuildTarget = "./cmd/resource-advisor"
	golang.BuildMatrix = append(goutil.DefaultBuildMatrix, goutil.BuildPlatform{OS: "windows", Arch: "amd64"})
}
