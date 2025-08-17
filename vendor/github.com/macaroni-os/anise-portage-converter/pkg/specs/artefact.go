/*
Copyright © 2021-2023 Macaroni OS Linux
See AUTHORS and LICENSE for the license details and contributors.
*/
package specs

import (
	"errors"
)

func NewPortageConverterArtefact(pkg string) *PortageConverterArtefact {
	return &PortageConverterArtefact{
		Tree:            "",
		IgnoreBuildDeps: false,
		Packages:        []string{pkg},
		Annotations:     make(map[string]interface{}, 0),
		OverrideVersion: "",
	}
}

func (a *PortageConverterArtefact) GetPackages() []string                  { return a.Packages }
func (a *PortageConverterArtefact) GetTree() string                        { return a.Tree }
func (a *PortageConverterArtefact) GetAnnotations() map[string]interface{} { return a.Annotations }

func (a *PortageConverterArtefact) HasRuntimeMutations() bool {
	ans := false
	if len(a.Mutations.RuntimeDeps.Packages) > 0 || len(a.Mutations.Uses) > 0 ||
		len(a.Mutations.Provides) > 0 || len(a.Mutations.Conflicts) > 0 {
		ans = true
	}
	return ans
}

func (a *PortageConverterArtefact) HasConflict(p *PortageConverterPkg) bool {
	ans := false
	if p != nil && len(a.Mutations.Conflicts) > 0 {
		for _, c := range a.Mutations.Conflicts {
			if c.Name == p.Name && c.Category == p.Category {
				ans = true
				break
			}
		}
	}
	return ans
}

func (a *PortageConverterArtefact) HasBuildtimeMutations() bool {
	if len(a.Mutations.BuildTimeDeps.Packages) > 0 {
		return true
	}
	return false
}

func (a *PortageConverterArtefact) HasOverrideVersion(pkg string) bool {
	ans := false
	hasPkg := false

	for _, p := range a.Packages {
		if p == pkg {
			hasPkg = true
			break
		}
	}

	if hasPkg && a.OverrideVersion != "" {
		ans = true
	}

	return ans
}

func (a *PortageConverterArtefact) GetOverrideVersion() string {
	return a.OverrideVersion
}

func (a *PortageConverterArtefact) IgnoreBuildtime(pkg string) bool {
	_, ans := a.MapIgnoreBuildtime[pkg]
	return ans
}

func (a *PortageConverterArtefact) IgnoreRuntime(pkg string) bool {
	_, ans := a.MapIgnoreRuntime[pkg]
	return ans
}

func (s *PortageConverterArtefact) HasRuntimeReplacement(pkg string) bool {
	_, ans := s.MapReplacementsRuntime[pkg]
	return ans
}

func (s *PortageConverterArtefact) HasBuildtimeReplacement(pkg string) bool {
	_, ans := s.MapReplacementsBuildtime[pkg]
	return ans
}

func (s *PortageConverterArtefact) GetBuildtimeReplacement(pkg string) (*PortageConverterReplacePackage, error) {
	ans, ok := s.MapReplacementsBuildtime[pkg]
	if ok {
		return ans, nil
	}
	return ans, errors.New("No replacement found for key " + pkg)
}

func (s *PortageConverterArtefact) GetRuntimeReplacement(pkg string) (*PortageConverterReplacePackage, error) {
	ans, ok := s.MapReplacementsRuntime[pkg]
	if ok {
		return ans, nil
	}
	return ans, errors.New("No replacement found for key " + pkg)
}
