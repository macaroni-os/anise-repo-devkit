/*
	Copyright © 2021-2023 Macaroni OS Linux
	See AUTHORS and LICENSE for the license details and contributors.
*/

package converter

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/macaroni-os/anise-portage-converter/pkg/qdepends"
	"github.com/macaroni-os/anise-portage-converter/pkg/reposcan"
	"github.com/macaroni-os/anise-portage-converter/pkg/specs"
	"github.com/macaroni-os/anise-portage-converter/pkg/utils"

	luet_config "github.com/geaaru/luet/pkg/config"
	. "github.com/geaaru/luet/pkg/logger"
	luet_pkg "github.com/geaaru/luet/pkg/package"
	luet_tree "github.com/geaaru/luet/pkg/tree"
	gentoo "github.com/geaaru/pkgs-checker/pkg/gentoo"
)

var (
	BuildTime   string
	BuildCommit string
)

type PortageConverter struct {
	Config         *luet_config.LuetConfig
	Cache          map[string]*specs.PortageSolution
	ReciperBuild   luet_tree.Builder
	ReciperRuntime luet_tree.Builder
	Specs          *specs.PortageConverterSpecs
	TargetDir      string
	Solutions      []*specs.PortageSolution
	Backend        string
	Resolver       specs.PortageResolver

	WithPortagePkgs      bool
	Override             bool
	IgnoreMissingDeps    bool
	IgnoreWrongPackages  bool
	DisableStage2        bool
	DisableStage3        bool
	DisableStage4        bool
	DisableConflicts     bool
	UsingLayerForRuntime bool
	ContinueWithError    bool
	CheckUpdate4Deps     bool
	SkipRDepsGeneration  bool
	DisabledUseFlags     []string
	TreePaths            []string
	FilteredPackages     []string
}

func NewPortageConverter(targetDir, backend string) *PortageConverter {
	return &PortageConverter{
		// TODO: we use it as singleton
		Config:               luet_config.LuetCfg,
		Cache:                make(map[string]*specs.PortageSolution, 0),
		ReciperBuild:         luet_tree.NewCompilerRecipe(luet_pkg.NewInMemoryDatabase(false)),
		ReciperRuntime:       luet_tree.NewInstallerRecipe(luet_pkg.NewInMemoryDatabase(false)),
		TargetDir:            targetDir,
		Backend:              backend,
		Override:             false,
		IgnoreMissingDeps:    false,
		IgnoreWrongPackages:  false,
		ContinueWithError:    false,
		DisableConflicts:     false,
		CheckUpdate4Deps:     false,
		UsingLayerForRuntime: false,
		SkipRDepsGeneration:  false,
		DisabledUseFlags:     []string{},
		TreePaths:            []string{},
		FilteredPackages:     []string{},
	}
}

func (pc *PortageConverter) IsFilteredPackage(pkg string) (bool, error) {
	ans := true

	if len(pc.FilteredPackages) == 0 {
		return false, nil
	}

	gp, err := gentoo.ParsePackageStr(pkg)
	if err != nil {
		return ans, err
	}

	for _, f := range pc.FilteredPackages {

		gpf, err := gentoo.ParsePackageStr(f)
		if err != nil {
			return ans, err
		}

		if gpf.GetPackageName() != gp.GetPackageName() {
			continue
		}

		admitted, err := gpf.Admit(gp)
		if err != nil {
			return ans, err
		}

		if admitted {
			ans = false
			break
		}
	}

	return ans, nil
}

func (pc *PortageConverter) LoadTrees(treePath []string) error {

	if len(pc.Specs.TreePaths) > 0 {
		// Review path of the tree from the specs
		// in order to use the basedir of the spec
		// file as start point for the tree to load.
		absPath, err := filepath.Abs(path.Dir(pc.Specs.File))
		if err != nil {
			return fmt.Errorf("error on retrieve abs path of sepcs file %s: %s",
				pc.Specs.File, err.Error())
		}

		for _, t := range pc.Specs.TreePaths {
			tpath := filepath.Join(absPath, t)
			err := pc.loadTree(tpath)
			if err != nil {
				return err
			}
		}

	}

	// Load trees
	for _, t := range treePath {
		err := pc.loadTree(t)
		if err != nil {
			return err
		}
	}

	return nil
}

func (pc *PortageConverter) loadTree(t string) error {
	if utils.Exists(t) {
		InfoC(fmt.Sprintf(":evergreen_tree: Loading tree %s...", t))
		err := pc.ReciperBuild.Load(t)
		if err != nil {
			return errors.New("Error on load tree" + err.Error())
		}
		err = pc.ReciperRuntime.Load(t)
		if err != nil {
			return errors.New("Error on load tree" + err.Error())
		}
	} else {
		Warning(fmt.Sprintf(":warning: Ignoring tree %s not available.", t))
	}

	return nil
}

func (pc *PortageConverter) LoadRules(file string) error {
	spec, err := specs.LoadSpecsFile(file)
	if err != nil {
		return err
	}

	pc.Specs = spec

	if spec.BuildTmplFile == "" {
		return errors.New("No build template file defined")
	}

	return nil
}

func (pc *PortageConverter) GetSpecs() *specs.PortageConverterSpecs { return pc.Specs }
func (pc *PortageConverter) GetFilteredPackages() []string          { return pc.FilteredPackages }

func (pc *PortageConverter) SetFilteredPackages(pkgs []string) {
	pc.FilteredPackages = pkgs
}

func (pc *PortageConverter) IsDep2Skip(pkg *gentoo.GentooPackage, buildDep bool) bool {

	for _, skipPkg := range pc.Specs.SkippedResolutions.Packages {
		if skipPkg.Name == pkg.Name && skipPkg.Category == pkg.Category {
			return true
		}
	}

	for _, cat := range pc.Specs.SkippedResolutions.Categories {
		if cat == pkg.Category {
			return true
		}
	}

	if buildDep {
		for _, cat := range pc.Specs.SkippedResolutions.BuildCategories {
			if cat == pkg.Category {
				return true
			}
		}
	}

	return false
}

func (pc *PortageConverter) AppendIfNotPresent(list []gentoo.GentooPackage, pkg gentoo.GentooPackage) []gentoo.GentooPackage {
	ans := list
	isPresent := false
	for _, p := range list {
		if p.Name == pkg.Name && p.Category == pkg.Category {
			isPresent = true
			break
		}
	}
	if !isPresent {
		ans = append(ans, pkg)
	}
	return ans
}

func (pc *PortageConverter) createSolution(pkg, treePath string, stack []string, artefact specs.PortageConverterArtefact) error {
	newVersion := false

	// Check if it's present artefact from map
	art, err := pc.Specs.GetArtefactByPackage(pkg)
	if err == nil {
		DebugC(fmt.Sprintf(
			"[%s] Using artefact from map. Uses disabled: %s, enabled: %s",
			pkg, art.Uses.Disabled, art.Uses.Enabled))
		// POST: use artefact from map.
		artefact = *art
	}

	// In order to identify a specific artefact of a package
	// with a specific version we need to have a way to describe
	// the package with an additional label that will be used
	// in the MapArtefacts field.
	// The idea is to use a <string> after the comma.
	// For example:
	// dev-libs/nodejs,nodejs20
	// This is needed to permit having multiple versions in the same
	// repo of a specific package and slot. Packages with different
	// slots doesn't suffer of this needed.

	pkgOriginalStr := pkg

	if strings.Index(pkg, ",") > 0 {
		pkg = strings.Split(pkg, ",")[0]
	}

	opts := &specs.PortageResolverOpts{
		EnableUseFlags:   artefact.Uses.Enabled,
		DisabledUseFlags: artefact.Uses.Disabled,
		Conditions:       artefact.Conditions,
	}

	if IsInStack(stack, pkg) {
		DebugC(fmt.Sprintf("Intercepted cycle dep for %s: %s", pkg, stack))
		DebugC(fmt.Sprintf("[%s] I skip cycle.", pkg))
		// TODO: Is this correct?
		return nil
	}

	gp, err := gentoo.ParsePackageStr(pkg)
	// Avoid to resolve it if it's skipped. Workaround to qdepends problems.
	if err != nil {
		return err
	}

	if pc.IsDep2Skip(gp, false) {
		if len(stack) > 1 {
			DebugC(fmt.Sprintf("[%s] Skipped dependency %s", stack[len(stack)-1], pkg))
		} else {
			InfoC(fmt.Sprintf("[%s] Skipped dependency not in stack.", pkg))
		}
		return nil
	}

	solution, err := pc.Resolver.Resolve(pkg, opts)
	if err != nil {
		if pc.IgnoreWrongPackages {
			Warning(fmt.Sprintf("Error on resolve %s: %s. Ignoring it.", pkg, err.Error()))
			return nil
		} else {
			return errors.New(fmt.Sprintf("Error on resolve %s: %s", pkg, err.Error()))
		}
	}

	DebugC(fmt.Sprintf("[%s] rconflicts %d rdeps %d bconflicts %d bdpes %d",
		pkg, len(solution.RuntimeConflicts), len(solution.RuntimeDeps),
		len(solution.BuildConflicts), len(solution.BuildDeps)))

	stack = append(stack, pkg)

	cacheKey := fmt.Sprintf("%s/%s",
		specs.SanitizeCategory(solution.Package.Category, solution.Package.Slot),
		solution.Package.Name)

	if _, ok := pc.Cache[cacheKey]; ok {
		DebugC(fmt.Sprintf("Package %s already in cache.", pkg))
		// Nothing to do
		return nil
	}

	InfoC(GetAurora().Bold(fmt.Sprintf(":pizza: [%s] (%s) Creating solution ...", pkgOriginalStr, treePath)))

	pkgDir := fmt.Sprintf("%s/%s/%s/",
		filepath.Join(pc.TargetDir, treePath),
		solution.Package.Category, solution.Package.Name)
	if solution.Package.Slot != "0" {
		slot := solution.Package.Slot
		// Ignore sub-slot
		if strings.Contains(solution.Package.Slot, "/") {
			slot = solution.Package.Slot[0:strings.Index(slot, "/")]
		}

		pkgDir = fmt.Sprintf("%s/%s-%s/%s",
			filepath.Join(pc.TargetDir, treePath),
			solution.Package.Category, slot, solution.Package.Name)
	}

	if artefact.CustomPath != "" {
		pkgDir = filepath.Join(pc.TargetDir, treePath,
			artefact.CustomPath)
	}

	// Check if specs is already present. I don't check definition.yaml
	// because with collection packages could be inside collection file.
	pTarget := luet_pkg.NewPackage(solution.Package.Name, ">=0",
		[]*luet_pkg.DefaultPackage{},
		[]*luet_pkg.DefaultPackage{})
	pTarget.Category = specs.SanitizeCategory(
		solution.Package.Category,
		solution.Package.Slot,
	)

	p, _ := pc.ReciperRuntime.GetDatabase().FindPackages(pTarget)
	// PRE: we currently consider that a package is present only one time with a single
	//      version.
	if p != nil {

		var originPackage luet_pkg.Package
		for _, luetPkgTree := range p {

			gpTree, err := gentoo.ParsePackageStr(fmt.Sprintf("%s/%s-%s",
				gp.Category, gp.Name, luetPkgTree.GetVersion()))
			if err != nil {
				Error(fmt.Sprintf("[%s] Error parse existing pkg in tree: %s", pkg, err.Error()))
				return err
			}

			// If the artefact contains a custom path I need to check if the
			// path is the same
			if artefact.CustomPath != "" {
				pkgDirAbs, _ := filepath.Abs(pkgDir)
				if luetPkgTree.GetPath() != pkgDirAbs {
					DebugC(fmt.Sprintf("[%s-%s] package %s skipped because doesn't match with custom path (%s != %s).",
						solution.Package.GetPackageName(),
						solution.Package.GetPVR(),
						luetPkgTree.HumanReadableString(),
						luetPkgTree.GetPath(), pkgDirAbs,
					))
					continue
				}
			}

			gt, err := solution.Package.GreaterThanOrEqual(gpTree)
			if err != nil {
				Error(fmt.Sprintf("[%s] Error on check if package is greater then existing: %s", pkg, err.Error()))
				return err
			}

			if gt {
				InfoC(fmt.Sprintf("[%s-%s] package to upgrade (%s)", solution.Package.GetPackageName(),
					solution.Package.GetPVR(), gpTree.GetPVR()))

				originaPkgVersion, _ := luetPkgTree.GetLabels()["original.package.version"]

				// Ensure that is related to the package suffix not available
				// on luet version
				if originaPkgVersion == "" || originaPkgVersion != solution.Package.GetPVR() {
					newVersion = true
				}
			} else {
				DebugC(fmt.Sprintf("[%s-%s] package already updated: %s %v",
					solution.Package.GetPackageName(),
					solution.Package.GetPVR(),
					gpTree.GetPVR(),
					gt,
				))
				if pc.Override {
					newVersion = true
				} else {
					newVersion = false
				}
			}

			originPackage = luetPkgTree
		}

		if originPackage != nil {
			solution.PackageUpgraded = originPackage.(*luet_pkg.DefaultPackage)

			if !newVersion && !pc.Override {
				if !pc.CheckUpdate4Deps || len(stack) > 1 {
					// Nothing to do
					InfoC(fmt.Sprintf("Package %s already in tree and updated.", pkg))
					return nil
				}
			}

			solution.Upgrade = true
			solution.PackageDir = pkgDir
		} else {
			newVersion = true
			solution.PackageDir = pkgDir
		}

	} else {
		newVersion = true
	}

	// TODO: atm I handle build-dep and runtime-dep at the same
	//       way. I'm not sure if this correct.

	if p == nil || pc.Override || pc.CheckUpdate4Deps {

		// Check every build dependency
		var bdeps []gentoo.GentooPackage = make([]gentoo.GentooPackage, 0)
		for _, bdep := range solution.BuildDeps {

			DebugC(fmt.Sprintf("[%s] Analyzing buildtime dep %s...", pkg, bdep.GetPackageName()))

			if pc.IsDep2Skip(&bdep, true) {
				DebugC(fmt.Sprintf("[%s] Skipped dependency %s", pkg, bdep.GetPackageName()))
				continue
			}

			dep_str := fmt.Sprintf("%s/%s", bdep.Category, bdep.Name)
			if bdep.Slot != "0" {
				dep_str += ":" + bdep.Slot
			}

			// Check if there is a layer to use for the dependency
			if pc.Specs.HasBuildLayer(dep_str) {
				bLayer, _ := pc.Specs.GetBuildLayer(dep_str)
				gp := gentoo.GentooPackage{
					Name:     bLayer.Layer.Name,
					Category: bLayer.Layer.Category,
					Version:  ">=0",
					Slot:     "0",
				}
				DebugC(GetAurora().Bold(fmt.Sprintf("[%s] For dep %s found layer %s/%s.", pkg,
					dep_str, bLayer.Layer.Name, bLayer.Layer.Category)))
				bdeps = pc.AppendIfNotPresent(bdeps, gp)
				continue
			}

			dep := luet_pkg.NewPackage(bdep.Name, ">=0",
				[]*luet_pkg.DefaultPackage{},
				[]*luet_pkg.DefaultPackage{})
			dep.Category = specs.SanitizeCategory(bdep.Category, bdep.Slot)

			// Check if it's present the build dep on the tree
			p, _ := pc.ReciperBuild.GetDatabase().FindPackages(dep)
			if pc.CheckUpdate4Deps || p == nil {

				// Check if there is a runtime deps/provide for this
				p, _ := pc.ReciperRuntime.GetDatabase().FindPackages(dep)
				if p == nil {
					// Now we use the same treePath.
					err := pc.createSolution(dep_str, treePath, stack, artefact)
					if err != nil {
						return err
					}

					bdeps = pc.AppendIfNotPresent(bdeps, bdep)
				} else {

					DebugC(fmt.Sprintf("[%s] For buildtime dep %s is used package %s",
						pkg, bdep.GetPackageName(), p[0].HumanReadableString()))

					gp := gentoo.GentooPackage{
						Name:     p[0].GetName(),
						Category: p[0].GetCategory(),
						Version:  ">=0",
						Slot:     "0",
					}
					bdeps = pc.AppendIfNotPresent(bdeps, gp)
				}
			} else {
				DebugC(fmt.Sprintf("[%s] For build-time dep %s is used package %s",
					pkg, bdep.GetPackageName(), p[0].HumanReadableString()))

				gp := gentoo.GentooPackage{
					Name:     p[0].GetName(),
					Category: p[0].GetCategory(),
					Version:  ">=0",
					Slot:     "0",
				}
				bdeps = pc.AppendIfNotPresent(bdeps, gp)
			}

		}

		if len(bdeps) == 0 && pc.Specs.HasBuildLayer(cacheKey) {
			// Check if the packages is present to a layer
			bLayer, _ := pc.Specs.GetBuildLayer(cacheKey)
			gp := gentoo.GentooPackage{
				Name:     bLayer.Layer.Name,
				Category: bLayer.Layer.Category,
				Version:  ">=0",
				Slot:     "0",
			}
			bdeps = pc.AppendIfNotPresent(bdeps, gp)

			DebugC(fmt.Sprintf("[%s] For build-time using only layer %s",
				pkg, gp.GetPackageName()))
		}

		solution.BuildDeps = bdeps

		// Check buildtime conflicts
		var bconflicts []gentoo.GentooPackage = make([]gentoo.GentooPackage, 0)
		if !pc.DisableConflicts {
			for _, bconflict := range solution.BuildConflicts {

				DebugC(fmt.Sprintf("[%s] Analyzing buildtime conflict %s...",
					pkg, bconflict.GetPackageName()))

				if pc.IsDep2Skip(&bconflict, true) {
					DebugC(fmt.Sprintf("[%s] Skipped dependency %s", pkg, bconflict.GetPackageName()))
					continue
				}

				gp := gentoo.GentooPackage{
					Name:     bconflict.Name,
					Category: specs.SanitizeCategory(bconflict.Category, bconflict.Slot),
					Version:  ">=0",
					Slot:     "0",
				}

				if bconflict.Condition == gentoo.PkgCondNotLess {
					gp.Version = fmt.Sprintf("<%s", bconflict.Version)
				} else if bconflict.Condition == gentoo.PkgCondNotGreater {
					gp.Version = fmt.Sprintf(">%s", bconflict.Version)
				}
				bconflicts = pc.AppendIfNotPresent(bconflicts, gp)
			}
		}
		solution.BuildConflicts = bconflicts

		// Check every runtime deps
		var rdeps []gentoo.GentooPackage = make([]gentoo.GentooPackage, 0)
		for _, rdep := range solution.RuntimeDeps {

			DebugC(fmt.Sprintf("[%s] Analyzing runtime dep %s...", pkg, rdep.GetPackageName()))

			if pc.IsDep2Skip(&rdep, false) {
				DebugC(fmt.Sprintf("[%s] Skipped dependency %s", pkg, rdep.GetPackageName()))
				continue
			}

			dep_str := fmt.Sprintf("%s/%s", rdep.Category, rdep.Name)
			if rdep.Slot != "0" {
				dep_str += ":" + rdep.Slot
			}

			dep := luet_pkg.NewPackage(rdep.Name, ">=0",
				[]*luet_pkg.DefaultPackage{},
				[]*luet_pkg.DefaultPackage{})
			dep.Category = specs.SanitizeCategory(rdep.Category, rdep.Slot)

			// Check if there is a layer to use for the dependency
			if pc.UsingLayerForRuntime && pc.Specs.HasBuildLayer(dep_str) {
				bLayer, _ := pc.Specs.GetBuildLayer(dep_str)
				gp := gentoo.GentooPackage{
					Name:     bLayer.Layer.Name,
					Category: bLayer.Layer.Category,
					Version:  ">=0",
					Slot:     "0",
				}
				DebugC(GetAurora().Bold(fmt.Sprintf(
					"[%s] For runtime dep %s found layer %s/%s.", pkg,
					dep_str, bLayer.Layer.Name, bLayer.Layer.Category)))
				rdeps = pc.AppendIfNotPresent(rdeps, gp)
				continue
			}

			// Check if it's present the build dep on the tree
			p, _ := pc.ReciperRuntime.GetDatabase().FindPackages(dep)
			if p == nil || pc.CheckUpdate4Deps {

				if !pc.SkipRDepsGeneration {
					dep_str := fmt.Sprintf("%s/%s", rdep.Category, rdep.Name)
					if rdep.Slot != "0" {
						dep_str += ":" + rdep.Slot
					}
					// Now we use the same treePath.
					err := pc.createSolution(dep_str, treePath, stack, artefact)
					if err != nil {
						return err
					}
				}

				rdeps = pc.AppendIfNotPresent(rdeps, rdep)

			} else {
				// TODO: handle package list in a better way
				DebugC(fmt.Sprintf("[%s] For runtime dep %s is used package %s",
					pkg, rdep.GetPackageName(), p[0].HumanReadableString()))

				gp := gentoo.GentooPackage{
					Name:     p[0].GetName(),
					Category: p[0].GetCategory(),
					Version:  ">=0",
					Slot:     "0",
				}
				rdeps = pc.AppendIfNotPresent(rdeps, gp)
			}
		}
		solution.RuntimeDeps = rdeps

		// Check runtime conflicts
		var rconflicts []gentoo.GentooPackage = make([]gentoo.GentooPackage, 0)
		if !pc.DisableConflicts {
			for _, rconflict := range solution.RuntimeConflicts {

				DebugC(fmt.Sprintf("[%s] Analyzing runtime conflict %s...",
					pkg, rconflict.GetPackageName()))

				if pc.IsDep2Skip(&rconflict, false) {
					DebugC(fmt.Sprintf("[%s] Skipped dependency %s", pkg, rconflict.GetPackageName()))
					continue
				}

				gp := gentoo.GentooPackage{
					Name:     rconflict.Name,
					Category: specs.SanitizeCategory(rconflict.Category, rconflict.Slot),
					Version:  ">=0",
					Slot:     "0",
				}

				if rconflict.Condition == gentoo.PkgCondNotLess {
					gp.Version = fmt.Sprintf("<%s", rconflict.Version)
				} else if rconflict.Condition == gentoo.PkgCondNotGreater {
					gp.Version = fmt.Sprintf(">%s", rconflict.Version)
				}

				rconflicts = pc.AppendIfNotPresent(rconflicts, gp)
			}
		}

		solution.RuntimeConflicts = rconflicts
		solution.PackageDir = pkgDir
	}

	if artefact.HasOverrideVersion(pkg) {
		DebugC(fmt.Sprintf("[%s] Override version with %s.",
			pkg, artefact.GetOverrideVersion()))

		solution.OverrideVersion = artefact.GetOverrideVersion()
	}

	if len(artefact.GetAnnotations()) > 0 {
		solution.Annotations = artefact.GetAnnotations()
	} else {
		solution.Annotations = *pc.Specs.GetGlobalAnnotations()
	}

	pc.Cache[cacheKey] = solution

	if !solution.Upgrade || (solution.Upgrade && newVersion) || pc.Override {
		pc.Solutions = append(pc.Solutions, solution)
	}

	return nil
}

func (pc *PortageConverter) createPortagePackage(pkg *specs.PortageSolution, originalPackage *luet_pkg.DefaultPackage) error {
	buildTmpl, err := NewLuetCompilationSpecSanitizedFromFile(pc.Specs.BuildPortageTmplFile)
	if err != nil {
		return errors.New("Error on load template: " + err.Error())
	}

	portagePkgDir := filepath.Join(pkg.PackageDir, "portage")
	err = os.MkdirAll(portagePkgDir, 0755)
	if err != nil {
		return err
	}

	defFile := filepath.Join(portagePkgDir, "definition.yaml")
	buildFile := filepath.Join(portagePkgDir, "build.yaml")

	dep := &luet_pkg.DefaultPackage{
		Name:     originalPackage.Name,
		Category: originalPackage.Category,
		Version:  originalPackage.Version,
	}
	// Set only required labels here
	labels := make(map[string]string, 0)
	labels["original.package.name"] = originalPackage.Labels["original.package.name"]
	labels["original.package.version"] = originalPackage.Labels["original.package.version"]
	labels["emerge.packages"] = originalPackage.Labels["emerge.packages"]
	labels["kit"] = originalPackage.Labels["kit"]

	pack := &luet_pkg.DefaultPackage{
		Name:     fmt.Sprintf("%s-portage", pkg.Package.Name),
		Category: originalPackage.Category,
		Version:  originalPackage.Version,
		Labels:   labels,
	}
	pack.Requires([]*luet_pkg.DefaultPackage{dep})

	// Write definition.yaml
	err = luet_tree.WriteDefinitionFile(pack, defFile)
	if err != nil {
		return err
	}

	buildPack, _ := buildTmpl.Clone()
	buildPack.AddRequires([]*luet_pkg.DefaultPackage{dep})
	err = buildPack.WriteBuildDefinition(buildFile)
	if err != nil {
		return err
	}

	return nil
}

func (pc *PortageConverter) InitConverter(showBanner bool) error {
	// Initialize resolver
	if pc.Backend == "reposcan" {
		if len(pc.Specs.ReposcanSources) == 0 {
			return errors.New("No reposcan sources defined!")
		}

		resolver := reposcan.NewRepoScanResolver()
		resolver.JsonSources = pc.Specs.ReposcanSources
		resolver.SetIgnoreMissingDeps(pc.IgnoreMissingDeps)
		resolver.SetDepsWithSlot(pc.Specs.ReposcanRequiresWithSlot)
		resolver.SetDisabledUseFlags(pc.Specs.ReposcanDisabledUseFlags)
		resolver.SetDisabledKeywords(pc.Specs.ReposcanDisabledKeywords)
		resolver.SetContinueWithError(pc.ContinueWithError)
		resolver.SetAllowEmptyKeywords(pc.Specs.ReposcanAllowEmpyKeywords)
		if showBanner {
			InfoC(fmt.Sprintf("Using dependency with slot on category: %v",
				resolver.GetDepsWithSlot()))
			InfoC(fmt.Sprintf("Disabled keywords: %s",
				GetAurora().Bold(resolver.GetDisabledKeywords())))
			InfoC(fmt.Sprintf("Disabled USE: %s",
				GetAurora().Bold(resolver.GetDisabledUseFlags())))
		}
		err := resolver.LoadJsonFiles(showBanner)
		if err != nil {
			return err
		}

		if len(pc.Specs.ReposcanConstraints.Packages) > 0 {
			resolver.Constraints = pc.Specs.ReposcanConstraints.Packages
		}

		err = resolver.BuildMap()
		if err != nil {
			return err
		}

		pc.Resolver = resolver
	} else {
		pc.Resolver = qdepends.NewQDependsResolver()
	}

	// Create artefacts map
	pc.Specs.GenerateArtefactsMap()
	pc.Specs.GenerateReplacementsMap()
	pc.Specs.GenerateBuildLayerMap()

	return nil
}

func (pc *PortageConverter) Generate() error {
	// Load Build template file
	buildTmpl, err := NewLuetCompilationSpecSanitizedFromFile(pc.Specs.BuildTmplFile)
	if err != nil {
		return errors.New("Error on load template: " + err.Error())
	}

	err = pc.InitConverter(true)
	if err != nil {
		return errors.New("Error on initialize converter: " + err.Error())
	}

	// Resolve all packages
	for _, artefact := range pc.Specs.GetArtefacts() {
		for _, pkg := range artefact.GetPackages() {

			filtered, err := pc.IsFilteredPackage(pkg)
			if err != nil {
				return err
			}

			if filtered {
				DebugC(fmt.Sprintf("[%s] Filtered package. I will ignore it.", pkg))
				continue
			}

			DebugC(fmt.Sprintf("Analyzing package %s...", pkg))
			err = pc.createSolution(pkg, artefact.GetTree(), []string{}, artefact)
			if err != nil {
				return err
			}
		}
	}

	// Stage1: Write new specs without analyzing requires for build / runtime.
	for _, pkg := range pc.Solutions {

		InfoC(fmt.Sprintf(
			":cake: Processing package %s-%s", pkg.Package.GetPackageName(), pkg.Package.GetPVR()))

		defFile := filepath.Join(pkg.PackageDir, "definition.yaml")
		buildFile := filepath.Join(pkg.PackageDir, "build.yaml")
		finalizerFile := filepath.Join(pkg.PackageDir, "finalize.yaml")

		// Convert solution to luet package
		pack := pkg.ToPack(true)

		if pkg.Upgrade && !pc.Override {

			// The package could be in another package dir
			defFile = filepath.Join(pkg.PackageUpgraded.GetPath(), "definition.yaml")
			buildFile = filepath.Join(pkg.PackageUpgraded.GetPath(), "build.yaml")
			finalizerFile = filepath.Join(pkg.PackageUpgraded.GetPath(), "finalize.yaml")

			InfoC(fmt.Sprintf("[%s-%s] package to upgrade", pkg.Package.GetPackageName(),
				pkg.Package.GetPVR()))

			// Copy annotations and update the keys
			annotations := pkg.PackageUpgraded.GetAnnotations()
			if annotations == nil {
				annotations = make(map[string]interface{}, 1)
			}

			for k, v := range pack.GetAnnotations() {
				annotations[k] = v
			}

			for k, v := range annotations {
				pkg.PackageUpgraded.AddAnnotation(k, v)
			}

			for k, v := range pack.GetLabels() {
				pkg.PackageUpgraded.AddLabel(k, v)
			}

			gpTree, _ := gentoo.ParsePackageStr(fmt.Sprintf("%s/%s-%s",
				pkg.PackageUpgraded.Category, pkg.PackageUpgraded.Name,
				pkg.PackageUpgraded.GetVersion()))

			if gpTree.Version == pack.GetVersion() {
				// Bump a new build version from the existing version.
				pack.SetVersion(pkg.PackageUpgraded.GetVersion())
				err = pack.BumpBuildVersion()
				if err != nil {
					return err
				}
			}
			pkg.PackageUpgraded.SetVersion(pack.GetVersion())

			pack = pkg.PackageUpgraded
			pack.SetPath("")

			// Write definition.yaml
			err = luet_tree.WriteDefinitionFile(pack, defFile)
			if err != nil {
				return err
			}

			// Check if artefact is in map
			artefact, err := pc.Specs.GetArtefactByPackage(pkg.Package.GetPackageNameWithSlot())
			if err != nil {
				if pkg.Package.Slot != "" {
					artefact, err = pc.Specs.GetArtefactByPackage(pkg.Package.GetPackageName())
				}
			}
			if artefact != nil {
				// Check if there is a finalize to write
				if artefact.Finalize.IsValid() {
					err = artefact.Finalize.WriteFinalize(finalizerFile)
					if err != nil {
						return errors.New(
							"Error on create finalize.yaml: " + err.Error(),
						)
					}
				}
			}

		} else {
			err := os.MkdirAll(pkg.PackageDir, 0755)
			if err != nil {
				return fmt.Errorf("error on create dir %s for package %s: %s",
					pkg.PackageDir, pkg.Package.GetPackageNameWithSlot(), err.Error())
			}

			// Write definition.yaml
			err = luet_tree.WriteDefinitionFile(pack, defFile)
			if err != nil {
				return err
			}

			// Check if artefact is in map
			ignoreBuildDeps := false
			artefact, err := pc.Specs.GetArtefactByPackage(pkg.Package.GetPackageNameWithSlot())
			if err != nil {
				if pkg.Package.Slot != "" {
					artefact, err = pc.Specs.GetArtefactByPackage(pkg.Package.GetPackageName())
				}
			}
			if artefact != nil {
				ignoreBuildDeps = artefact.IgnoreBuildDeps

				// Check if there is a finalize to write
				if artefact.Finalize.IsValid() {
					err = artefact.Finalize.WriteFinalize(finalizerFile)
					if err != nil {
						return errors.New(
							"Error on create finalize.yaml: " + err.Error(),
						)
					}
				}
			}

			// create build.yaml
			bPack := pkg.ToPack(false)
			buildPack, _ := buildTmpl.Clone()
			if !ignoreBuildDeps {
				buildPack.AddRequires(bPack.PackageRequires)
			} else {
				DebugC(fmt.Sprintf(
					"[%s] :warning: Ignoring all build deps..",
					pkg.Package.GetPackageName()))
			}
			buildPack.AddConflicts(bPack.PackageConflicts)

			err = buildPack.WriteBuildDefinition(buildFile)
			if err != nil {
				return err
			}

			if pc.WithPortagePkgs {
				err = pc.createPortagePackage(pkg, pack)
				if err != nil {
					return err
				}
			}
		}

	} // end pc.Solutions

	InfoC(GetAurora().Bold(fmt.Sprintf(
		"Stage1 Completed: generated %d packages.", len(pc.Solutions))))

	// Stage2 apply replacements and mutations
	if pc.DisableStage2 {
		InfoC(GetAurora().Bold(fmt.Sprintf("Stage2 Disabled.")))
	} else {
		err = pc.Stage2()
		if err != nil {
			return err
		}
	}

	// Stage3: Reload tree and drop redundant dependencies
	if pc.DisableStage3 {
		InfoC(GetAurora().Bold(fmt.Sprintf("Stage3 Disabled.")))
	} else {
		err = pc.Stage3()
		if err != nil {
			return err
		}
	}

	// Stage4: Reload tree and review build dependencies to reduce diamond deps.
	if pc.DisableStage4 {
		InfoC(GetAurora().Bold(fmt.Sprintf("Stage4 Disabled.")))
	} else {
		err = pc.Stage4()
		if err != nil {
			return err
		}
	}

	return nil
}
