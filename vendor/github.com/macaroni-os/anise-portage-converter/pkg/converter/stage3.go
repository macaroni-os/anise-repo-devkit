/*
Copyright © 2021-2023 Macaroni OS Linux
See AUTHORS and LICENSE for the license details and contributors.
*/
package converter

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	. "github.com/geaaru/luet/pkg/logger"
	luet_pkg "github.com/geaaru/luet/pkg/package"
	luet_tree "github.com/geaaru/luet/pkg/tree"
	"github.com/macaroni-os/anise-portage-converter/pkg/specs"
)

func (pc *PortageConverter) Stage3() error {

	InfoC(GetAurora().Bold("Stage3 Starting..."))

	if len(pc.Solutions) == 0 {
		InfoC(GetAurora().Bold("Stage3: No solutions to elaborate. Nothing to do."))
		return nil
	}

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
		runtimeDepsRemoved := 0
		runtimeConflictsRemoved := 0
		buildtimeDepsRemoved := 0
		buildtimeConflictsRemoved := 0
		resolvedRuntimeDeps := []*luet_pkg.DefaultPackage{}
		resolvedBuildtimeDeps := []*luet_pkg.DefaultPackage{}
		resolvedRuntimeConflicts := []*luet_pkg.DefaultPackage{}
		resolvedBuildConflicts := []*luet_pkg.DefaultPackage{}

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
			return err
		}

		// Ensure to use use flags from the tree. Could be mutated.
		pkg.Package.UseFlags = pReciper.GetUses()

		// Drop conflicts not present on tree
		conflicts := pReciper.GetConflicts()
		if len(conflicts) > 0 {

			for _, dep := range conflicts {
				pp, _ := pc.ReciperBuild.GetDatabase().FindPackages(
					&luet_pkg.DefaultPackage{
						Name:     dep.GetName(),
						Category: dep.GetCategory(),
						Version:  dep.GetVersion(),
					},
				)

				if pp == nil || len(pp) == 0 {
					pp, _ := pc.ReciperRuntime.GetDatabase().FindPackages(
						&luet_pkg.DefaultPackage{
							Name:     dep.GetName(),
							Category: dep.GetCategory(),
							Version:  dep.GetVersion(),
						},
					)
					if pp == nil || len(pp) == 0 {
						InfoC(fmt.Sprintf("[%s/%s-%s] Dropping buildtime conflict %s/%s not available in tree.",
							pack.GetCategory(), pack.GetName(), pack.GetVersion(),
							dep.GetCategory(), dep.GetName(),
						))
						buildtimeConflictsRemoved++
					} else {
						resolvedBuildConflicts = append(resolvedBuildConflicts, dep)
					}
				}
			}

			if len(resolvedBuildConflicts) != len(conflicts) {
				updateBuildDeps = true
			}

		} // end len(conflicts)

		deps := pReciper.GetRequires()
		if len(deps) > 1 {

			for idx, dep := range deps {
				alreadyInjected := false

				for idx2, d2 := range deps {
					if idx2 == idx {
						continue
					}

					d2pkgs, err := pc.ReciperBuild.GetDatabase().FindPackages(
						&luet_pkg.DefaultPackage{
							Name:     d2.GetName(),
							Category: d2.GetCategory(),
							Version:  ">=0",
						},
					)
					if err != nil {
						return err
					}

					if len(d2pkgs) > 0 {

						for _, d3 := range d2pkgs[0].GetRequires() {
							if d3.GetName() == dep.GetName() && d3.GetCategory() == dep.GetCategory() {
								alreadyInjected = true

								DebugC(fmt.Sprintf("[%s/%s-%s] Dropping buildtime dep %s/%s available in %s/%s",
									pack.GetCategory(), pack.GetName(), pack.GetVersion(),
									dep.GetCategory(), dep.GetName(),
									d2.GetCategory(), d2.GetName(),
								))
								buildtimeDepsRemoved++
								goto next_dep
							}
						}
					} else {
						Warning(fmt.Sprintf(
							"[%s/%s-%s] No dependencies find for dep %s/%s - %s/%s",
							pack.GetCategory(), pack.GetName(), pack.GetVersion(),
							dep.GetCategory(), dep.GetName(),
							d2.GetCategory(), d2.GetName()))
					}

				}

			next_dep:

				if !alreadyInjected {
					resolvedBuildtimeDeps = append(resolvedBuildtimeDeps, dep)
				}

			} // end for idx, dep

			if len(resolvedBuildtimeDeps) != len(deps) {
				updateBuildDeps = true
			}

		} else {

			DebugC(fmt.Sprintf("[%s/%s-%s] Only one buildtime dep present. Nothing to do.",
				pack.GetCategory(), pack.GetName(), pack.GetVersion()))

			resolvedBuildtimeDeps = deps
		}

		// Check runtime requires
		pReciper, err = pc.ReciperRuntime.GetDatabase().FindPackage(luetPkg)
		if err != nil {
			return err
		}

		// Drop conflicts not present on tree
		conflicts = pReciper.GetConflicts()
		if len(conflicts) > 0 {

			// Check for artefact replacements
			art, _ := pc.Specs.GetArtefactByPackage(pkg.Package.GetPackageNameWithSlot())

			for _, dep := range conflicts {

				pcpkg := &specs.PortageConverterPkg{
					Name:     dep.GetName(),
					Category: dep.GetCategory(),
					Version:  dep.GetVersion(),
				}

				// Check if the conflict is defined at specs level and keep it if
				// it's present.
				if art != nil && art.HasRuntimeMutations() && art.HasConflict(pcpkg) {
					resolvedRuntimeConflicts = append(resolvedRuntimeConflicts, dep)
					continue
				}

				pp, _ := pc.ReciperRuntime.GetDatabase().FindPackages(
					&luet_pkg.DefaultPackage{
						Name:     dep.GetName(),
						Category: dep.GetCategory(),
						Version:  dep.GetVersion(),
					},
				)
				if pp == nil || len(pp) == 0 {

					InfoC(fmt.Sprintf("[%s/%s-%s] Dropping runtime conflict %s/%s not available in tree.",
						pack.GetCategory(), pack.GetName(), pack.GetVersion(),
						dep.GetCategory(), dep.GetName(),
					))
					runtimeConflictsRemoved++
				} else {
					resolvedRuntimeConflicts = append(resolvedRuntimeConflicts, dep)
				}
			}

			if len(resolvedRuntimeConflicts) != len(conflicts) {
				updateRuntimeDeps = true
			}

		}

		deps = pReciper.GetRequires()
		if len(deps) > 1 {

			for idx, dep := range deps {
				alreadyInjected := false

				for idx2, d2 := range deps {
					if idx2 == idx {
						continue
					}

					d2pkgs, err := pc.ReciperRuntime.GetDatabase().FindPackages(
						&luet_pkg.DefaultPackage{
							Name:     d2.GetName(),
							Category: d2.GetCategory(),
							Version:  ">=0",
						},
					)
					if err != nil {
						return err
					}

					if len(d2pkgs) > 0 {

						for _, d3 := range d2pkgs[0].GetRequires() {
							if d3.GetName() == dep.GetName() && d3.GetCategory() == dep.GetCategory() {
								alreadyInjected = true

								DebugC(fmt.Sprintf("[%s/%s-%s] Dropping runtime dep %s/%s available in %s/%s",
									pack.GetCategory(), pack.GetName(), pack.GetVersion(),
									dep.GetCategory(), dep.GetName(),
									d2.GetCategory(), d2.GetName(),
								))
								runtimeDepsRemoved++
								goto next_rdep
							}
						}

					} else {
						Warning(fmt.Sprintf(
							"[%s/%s-%s] No dependencies find for dep %s/%s - %s/%s",
							pack.GetCategory(), pack.GetName(), pack.GetVersion(),
							dep.GetCategory(), dep.GetName(),
							d2.GetCategory(), d2.GetName()))
					}
				}

			next_rdep:

				if !alreadyInjected {
					resolvedRuntimeDeps = append(resolvedRuntimeDeps, dep)
				}

			} // end for idx, dep

			if len(resolvedRuntimeDeps) != len(deps) {
				updateRuntimeDeps = true
			}

		} else {

			DebugC(fmt.Sprintf("[%s/%s-%s] Only one runtime dep present. Nothing to do.",
				pack.GetCategory(), pack.GetName(), pack.GetVersion()))

			resolvedRuntimeDeps = deps

		}

		// Write definition
		if updateRuntimeDeps {

			defFile := filepath.Join(pkg.PackageDir, "definition.yaml")
			// Convert solution to luet package

			// Read current definition file
			dat, err := ioutil.ReadFile(defFile)
			if err != nil {
				return err
			}
			p, err := luet_pkg.DefaultPackageFromYaml(dat)
			if err != nil {
				return err
			}
			pack = &p
			pack.Requires(resolvedRuntimeDeps)
			pack.Conflicts(resolvedRuntimeConflicts)

			// Write definition.yaml
			err = luet_tree.WriteDefinitionFile(pack, defFile)
			if err != nil {
				return err
			}
		}

		// Write build.yaml
		if updateBuildDeps {

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

			err = buildPack.WriteBuildDefinition(buildFile)
			if err != nil {
				return err
			}

		}

		if updateBuildDeps || updateRuntimeDeps {
			InfoC(GetAurora().Bold(
				fmt.Sprintf(
					":angel: [%s/%s-%s] removed: r.deps %d, r.conflicts %d, b.deps %d, b.conflicts %d.",
					pack.GetCategory(), pack.GetName(), pack.GetVersion(),
					runtimeDepsRemoved, runtimeConflictsRemoved,
					buildtimeDepsRemoved, buildtimeConflictsRemoved,
				)))
		}

	}

	return nil
}
