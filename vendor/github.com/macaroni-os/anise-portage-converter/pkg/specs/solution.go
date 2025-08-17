/*
Copyright © 2021-2023 Macaroni OS Linux
See AUTHORS and LICENSE for the license details and contributors.
*/
package specs

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	luet_pkg "github.com/geaaru/luet/pkg/package"
	gentoo "github.com/geaaru/pkgs-checker/pkg/gentoo"
)

func (s *PortageSolution) SetLabel(k, v string) {
	if v != "" {
		s.Labels[k] = v
	}
}

func (s *PortageSolution) ToPack(runtime bool) *luet_pkg.DefaultPackage {

	version := s.Package.Version

	if s.OverrideVersion != "" {
		version = s.OverrideVersion
	} else {
		// TODO: handle particular use cases
		if strings.HasPrefix(s.Package.VersionSuffix, "_pre") {
			version = version + s.Package.VersionSuffix
		}

		if s.Package.VersionBuild != "" {
			version += "+" + s.Package.VersionBuild
		}
	}

	emergePackage := s.Package.GetPackageName()
	if s.Package.Slot != "0" {
		emergePackage = emergePackage + ":" + s.Package.Slot
	}

	labels := s.Labels
	labels["original.package.name"] = s.Package.GetPackageName()
	labels["original.package.version"] = s.Package.GetPVR()
	labels["original.package.slot"] = s.Package.Slot
	labels["emerge.packages"] = emergePackage
	labels["kit"] = s.Package.Repository

	useFlags := []string{}

	if len(s.Package.UseFlags) > 0 {
		// Avoid duplicated
		m := make(map[string]int, 0)
		for _, u := range s.Package.UseFlags {
			m[u] = 1
		}
		for k, _ := range m {
			useFlags = append(useFlags, k)
		}

		sort.Strings(useFlags)
	}

	if len(useFlags) == 0 {
		useFlags = nil
	}

	ans := &luet_pkg.DefaultPackage{
		Name:        s.Package.Name,
		Category:    SanitizeCategory(s.Package.Category, s.Package.Slot),
		Version:     version,
		UseFlags:    useFlags,
		Labels:      labels,
		License:     s.Package.License,
		Description: s.Description,
		Annotations: s.Annotations,
		Uri:         s.Uri,
	}

	deps := s.BuildDeps
	if runtime {
		deps = s.RuntimeDeps
	}

	for _, req := range deps {

		dep := &luet_pkg.DefaultPackage{
			Name:     req.Name,
			Category: SanitizeCategory(req.Category, req.Slot),
			UseFlags: req.UseFlags,
		}
		if req.Version != "" && req.Condition != gentoo.PkgCondNot &&
			req.Condition != gentoo.PkgCondAnyRevision &&
			req.Condition != gentoo.PkgCondMatchVersion &&
			req.Condition != gentoo.PkgCondEqual {

			// TODO: to complete
			dep.Version = fmt.Sprintf("%s%s%s",
				req.Condition.String(), req.Version, req.VersionSuffix)

		} else {
			dep.Version = ">=0"
		}

		ans.PackageRequires = append(ans.PackageRequires, dep)
	}

	if runtime && len(s.RuntimeConflicts) > 0 {

		for _, req := range s.RuntimeConflicts {

			dep := &luet_pkg.DefaultPackage{
				Name:     req.Name,
				Category: SanitizeCategory(req.Category, req.Slot),
				UseFlags: req.UseFlags,
			}

			dep.Version = req.Version

			// Skip itself. Maybe we need handle this case in a better way.
			if dep.Name == s.Package.Name && dep.Category == SanitizeCategory(s.Package.Category, s.Package.Slot) {
				continue
			}

			ans.PackageConflicts = append(ans.PackageConflicts, dep)
		}

	} else if !runtime && len(s.BuildConflicts) > 0 {

		for _, req := range s.BuildConflicts {

			dep := &luet_pkg.DefaultPackage{
				Name:     req.Name,
				Category: SanitizeCategory(req.Category, req.Slot),
				UseFlags: req.UseFlags,
			}
			dep.Version = req.Version

			ans.PackageConflicts = append(ans.PackageConflicts, dep)
		}

	}

	return ans
}

func (s *PortageSolution) String() string {
	data, _ := json.Marshal(*s)
	return string(data)
}

func SanitizeCategory(cat string, slot string) string {
	ans := cat
	if slot != "0" {
		// Ignore sub-slot
		if strings.Contains(slot, "/") {
			slot = slot[0:strings.Index(slot, "/")]
		}

		if slot != "0" && slot != "" {
			ans = fmt.Sprintf("%s-%s", cat, slot)
		}
	}
	return ans
}
