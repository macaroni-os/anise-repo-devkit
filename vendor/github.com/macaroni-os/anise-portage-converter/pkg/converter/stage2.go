/*
Copyright © 2021-2023 Macaroni OS Linux
See AUTHORS and LICENSE for the license details and contributors.
*/
package converter

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/macaroni-os/anise-portage-converter/pkg/specs"

	. "github.com/geaaru/luet/pkg/logger"
	luet_pkg "github.com/geaaru/luet/pkg/package"
	luet_tree "github.com/geaaru/luet/pkg/tree"
)

func (pc *PortageConverter) applyRuntimeMutations(pkg *luet_pkg.DefaultPackage, art *specs.PortageConverterArtefact) {
	v := ""

	if len(art.Mutations.RuntimeDeps.Packages) > 0 {
		rdeps := pkg.GetRequires()
		for _, p := range art.Mutations.RuntimeDeps.Packages {
			v = p.Version
			if v == "" {
				v = ">=0"
			}

			rdeps = append(rdeps, &luet_pkg.DefaultPackage{
				Name:     p.Name,
				Category: p.Category,
				Version:  v,
			})
		}

		pkg.Requires(rdeps)
	}

	if len(art.Mutations.Uses) > 0 {
		pkg.UseFlags = append(pkg.UseFlags, art.Mutations.Uses...)
	}

	if len(art.Mutations.Provides) > 0 {
		provides := pkg.GetProvides()
		for _, p := range art.Mutations.Provides {
			v = p.Version
			if v == "" {
				v = ">=0"
			}

			provides = append(provides, &luet_pkg.DefaultPackage{
				Name:     p.Name,
				Category: p.Category,
				Version:  v,
			})
		}
		pkg.SetProvides(provides)
	}

	if len(art.Mutations.Conflicts) > 0 {
		conflicts := pkg.GetConflicts()
		for _, p := range art.Mutations.Conflicts {
			v = p.Version
			if v == "" {
				v = ">=0"
			}

			conflicts = append(conflicts, &luet_pkg.DefaultPackage{
				Name:     p.Name,
				Category: p.Category,
				Version:  v,
			})
		}
		pkg.Conflicts(conflicts)
	}
}

func (pc *PortageConverter) applyBuildtimeMutations(pkg *LuetCompilationSpecSanitized, art *specs.PortageConverterArtefact) {
	if len(art.Mutations.BuildTimeDeps.Packages) > 0 {
		bdeps := []*luet_pkg.DefaultPackage{}
		v := ""
		for _, p := range art.Mutations.BuildTimeDeps.Packages {
			v = p.Version
			if v == "" {
				v = ">=0"
			}
			bdeps = append(bdeps, &luet_pkg.DefaultPackage{
				Name:     p.Name,
				Category: p.Category,
				Version:  v,
			})
		}
		pkg.AddRequires(bdeps)
	}
}

func (pc *PortageConverter) Stage2() error {

	InfoC(GetAurora().Bold("Stage2 Starting..."))

	// Reset reciper
	pc.ReciperBuild = luet_tree.NewCompilerRecipe(luet_pkg.NewInMemoryDatabase(false))
	pc.ReciperRuntime = luet_tree.NewInstallerRecipe(luet_pkg.NewInMemoryDatabase(false))

	err := pc.LoadTrees(pc.TreePaths)
	if err != nil {
		return err
	}

	for _, pkg := range pc.Solutions {

		if pkg.Upgrade && !pc.Override {
			continue
		}

		pack := pkg.ToPack(true)
		updateBuildDeps := false
		updateRuntimeDeps := false
		runtimeDepsReplaced := 0
		runtimeConflictsReplaced := 0
		runtimeDepsIgnored := 0
		buildtimeDepsIgnored := 0
		buildtimeDepsReplaced := 0
		buildtimeConflictsReplaced := 0
		resolvedRuntimeDeps := []*luet_pkg.DefaultPackage{}
		resolvedBuildtimeDeps := []*luet_pkg.DefaultPackage{}
		resolvedRuntimeConflicts := []*luet_pkg.DefaultPackage{}
		resolvedBuildConflicts := []*luet_pkg.DefaultPackage{}

		// Check for artefact replacements
		art, _ := pc.Specs.GetArtefactByPackage(pkg.Package.GetPackageNameWithSlot())

		// Check buildtime requires
		DebugC(GetAurora().Bold(fmt.Sprintf("[%s/%s-%s]",
			pack.GetCategory(), pack.GetName(), pack.GetVersion())),
			"Checking buildtime dependencies...")

		luetPkg := &luet_pkg.DefaultPackage{
			Name:     pack.GetName(),
			Category: pack.GetCategory(),
			Version:  pack.GetVersion(),
		}
		pReciper, err := pc.ReciperBuild.GetDatabase().FindPackage(luetPkg)
		if err != nil {
			return errors.New(fmt.Sprintf(
				"Error on retrieve package %s/%s-%s from tree: %s",
				pack.GetCategory(), pack.GetName(), pack.GetVersion(),
				err.Error(),
			))
		}

		// Drop conflicts not present on tree
		conflicts := pReciper.GetConflicts()
		if len(conflicts) > 0 {

			for _, dep := range conflicts {

				pkgstr := fmt.Sprintf("%s/%s", dep.GetCategory(), dep.GetName())

				// Check global replacements
				if pc.Specs.HasBuildtimeReplacement(pkgstr) {

					to, err := pc.Specs.GetBuildtimeReplacement(pkgstr)
					if err != nil {
						return err
					}

					resolvedBuildConflicts = append(resolvedBuildConflicts,
						&luet_pkg.DefaultPackage{
							Name:     to.To.Name,
							Category: to.To.Category,
							Version:  ">=0",
						})
					buildtimeConflictsReplaced++
					continue
				}

				// Check if dep must be ignored
				if art != nil && art.IgnoreBuildtime(pkgstr) {
					buildtimeDepsIgnored++
					continue
				}

				if art != nil && art.HasBuildtimeReplacement(pkgstr) {
					to, err := art.GetBuildtimeReplacement(pkgstr)
					if err != nil {
						return err
					}

					resolvedBuildConflicts = append(resolvedBuildConflicts,
						&luet_pkg.DefaultPackage{
							Name:     to.To.Name,
							Category: to.To.Category,
							Version:  ">=0",
						})

					buildtimeConflictsReplaced++
					continue
				}

				resolvedBuildConflicts = append(resolvedBuildConflicts, dep)
			}

			if buildtimeConflictsReplaced > 0 || buildtimeDepsIgnored > 0 {
				updateBuildDeps = true
			}

		}

		deps := pReciper.GetRequires()
		if len(deps) > 0 {

			for _, dep := range deps {

				pkgstr := fmt.Sprintf("%s/%s", dep.GetCategory(), dep.GetName())
				// Check global replacements
				if pc.Specs.HasBuildtimeReplacement(pkgstr) {

					to, err := pc.Specs.GetBuildtimeReplacement(pkgstr)
					if err != nil {
						return err
					}

					resolvedBuildtimeDeps = append(resolvedBuildtimeDeps,
						&luet_pkg.DefaultPackage{
							Name:     to.To.Name,
							Category: to.To.Category,
							Version:  ">=0",
						})
					buildtimeDepsReplaced++
					continue
				}

				// Check if dep must be ignored
				if art != nil && art.IgnoreBuildtime(pkgstr) {
					buildtimeDepsIgnored++
					continue
				}

				if art != nil && art.HasBuildtimeReplacement(pkgstr) {
					to, err := art.GetBuildtimeReplacement(pkgstr)
					if err != nil {
						return err
					}

					resolvedBuildtimeDeps = append(resolvedBuildtimeDeps,
						&luet_pkg.DefaultPackage{
							Name:     to.To.Name,
							Category: to.To.Category,
							Version:  ">=0",
						})

					buildtimeDepsReplaced++
					continue
				}

				resolvedBuildtimeDeps = append(resolvedBuildtimeDeps, dep)
			}

			if buildtimeDepsReplaced > 0 || buildtimeDepsIgnored > 0 {
				updateBuildDeps = true
			}

		}

		// Check runtime requires
		pReciper, err = pc.ReciperRuntime.GetDatabase().FindPackage(luetPkg)
		if err != nil {
			return err
		}

		// Check runtime requires
		DebugC(GetAurora().Bold(fmt.Sprintf("[%s/%s-%s]",
			pack.GetCategory(), pack.GetName(), pack.GetVersion())),
			"Checking runtime dependencies...")

		// Drop conflicts not present on tree
		conflicts = pReciper.GetConflicts()
		if len(conflicts) > 0 {
			for _, dep := range conflicts {

				pkgstr := fmt.Sprintf("%s/%s", dep.GetCategory(), dep.GetName())
				// Check global replacements
				if pc.Specs.HasRuntimeReplacement(pkgstr) {

					to, err := pc.Specs.GetRuntimeReplacement(pkgstr)
					if err != nil {
						return err
					}

					resolvedRuntimeConflicts = append(resolvedRuntimeConflicts,
						&luet_pkg.DefaultPackage{
							Name:     to.To.Name,
							Category: to.To.Category,
							Version:  ">=0",
						})
					runtimeConflictsReplaced++
					continue
				}

				// Check if dep must be ignored
				if art != nil && art.IgnoreRuntime(pkgstr) {
					runtimeDepsIgnored++
					continue
				}

				if art != nil && art.HasRuntimeReplacement(pkgstr) {
					to, err := art.GetRuntimeReplacement(pkgstr)
					if err != nil {
						return err
					}

					resolvedRuntimeConflicts = append(resolvedRuntimeConflicts,
						&luet_pkg.DefaultPackage{
							Name:     to.To.Name,
							Category: to.To.Category,
							Version:  ">=0",
						})

					runtimeConflictsReplaced++
					continue
				}

				resolvedRuntimeConflicts = append(resolvedRuntimeConflicts, dep)
			}

			if runtimeConflictsReplaced > 0 || runtimeDepsIgnored > 0 {
				updateRuntimeDeps = true
			}

		}

		deps = pReciper.GetRequires()
		if len(deps) > 0 {

			for _, dep := range deps {

				pkgstr := fmt.Sprintf("%s/%s", dep.GetCategory(), dep.GetName())

				// Check global replacements
				if pc.Specs.HasRuntimeReplacement(pkgstr) {

					to, err := pc.Specs.GetRuntimeReplacement(pkgstr)
					if err != nil {
						return err
					}

					resolvedRuntimeDeps = append(resolvedRuntimeDeps,
						&luet_pkg.DefaultPackage{
							Name:     to.To.Name,
							Category: to.To.Category,
							Version:  ">=0",
						})
					runtimeDepsReplaced++
					continue
				}

				// Check if dep must be ignored
				if art != nil && art.IgnoreRuntime(pkgstr) {
					DebugC(GetAurora().Bold(fmt.Sprintf("[%s/%s-%s]",
						pack.GetCategory(), pack.GetName(), pack.GetVersion())),
						"Ignoring runtime dep "+pkgstr)
					runtimeDepsIgnored++
					continue
				}

				if art != nil && art.HasRuntimeReplacement(pkgstr) {
					to, err := art.GetRuntimeReplacement(pkgstr)
					if err != nil {
						return err
					}

					resolvedRuntimeDeps = append(resolvedRuntimeDeps,
						&luet_pkg.DefaultPackage{
							Name:     to.To.Name,
							Category: to.To.Category,
							Version:  ">=0",
						})

					runtimeDepsReplaced++
					continue
				}

				resolvedRuntimeDeps = append(resolvedRuntimeDeps, dep)
			}

			if runtimeDepsReplaced > 0 || runtimeDepsIgnored > 0 {
				updateRuntimeDeps = true
			}

		}

		// Write definition
		if updateRuntimeDeps || (art != nil && art.HasRuntimeMutations()) {

			defFile := filepath.Join(pkg.PackageDir, "definition.yaml")

			// Convert solution to luet package
			pack := pkg.ToPack(true)
			pack.Requires(resolvedRuntimeDeps)
			pack.Conflicts(resolvedRuntimeConflicts)

			if art != nil && art.HasRuntimeMutations() {
				pc.applyRuntimeMutations(pack, art)
			}

			// Write definition.yaml
			err = luet_tree.WriteDefinitionFile(pack, defFile)
			if err != nil {
				return err
			}
		}

		// Write build.yaml
		if updateBuildDeps || (art != nil && art.HasBuildtimeMutations()) {

			buildFile := filepath.Join(pkg.PackageDir, "build.yaml")
			// Load Build template file
			buildTmpl, err := NewLuetCompilationSpecSanitizedFromFile(pc.Specs.BuildTmplFile)
			if err != nil {
				return err
			}

			// create build.yaml
			buildPack, _ := buildTmpl.Clone()
			buildPack.Requires(resolvedBuildtimeDeps)
			buildPack.Conflicts(resolvedBuildConflicts)

			if art != nil && art.HasBuildtimeMutations() {
				pc.applyBuildtimeMutations(buildPack, art)
			}

			err = buildPack.WriteBuildDefinition(buildFile)
			if err != nil {
				return err
			}

		}

		if updateBuildDeps || updateRuntimeDeps {
			InfoC(GetAurora().Bold(
				fmt.Sprintf(
					":angel: [%s/%s-%s] replaced: r.deps %d, r.conflicts %d, r.ignored %d, b.deps %d, b.conflicts %d, b.ignored %d",
					pack.GetCategory(), pack.GetName(), pack.GetVersion(),
					runtimeDepsReplaced, runtimeConflictsReplaced, runtimeDepsIgnored,
					buildtimeDepsReplaced, buildtimeConflictsReplaced, buildtimeDepsIgnored,
				)))
		}

	}

	return nil
}
