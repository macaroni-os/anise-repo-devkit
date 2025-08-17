/*
Copyright © 2020-2025 Macaroni OS Linux
See AUTHORS and LICENSE for the license details and contributors.
*/
package specs

import (
	artifact "github.com/geaaru/luet/pkg/v2/compiler/types/artifact"
)

type AniseRDConfig struct {
	Cleaner AniseRDCCleaner `json:"cleaner,omitempty" yaml:"cleaner,omitempty"`
	List    AniseRDCList    `json:"list,omitempty" yaml:"list,omitempty"`
}

type AniseRDCCleaner struct {
	Excludes []string `json:"excludes,omitempty" yaml:"excludes,omitempty"`
}

type AniseRDCList struct {
	ExcludePkgs []AnisePackage `json:"exclude_pkgs,omitempty" yaml:"exclude_pkgs,omitempty"`
}

type AnisePackage struct {
	Name     string `json:"name" yaml:"name"`
	Category string `json:"category" yaml:"category"`
	Version  string `json:"version" yaml:"version"`
}

type RepoBackendHandler interface {
	GetFilesList() ([]string, error)
	GetMetadata(string) (*artifact.PackageArtifact, error)
	CleanFile(string) error
}
