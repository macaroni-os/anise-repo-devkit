/*
Copyright © 2021-2023 Macaroni OS Linux
See AUTHORS and LICENSE for the license details and contributors.
*/
package converter

import (
	"errors"
	"fmt"
	"path/filepath"

	cfg "github.com/geaaru/luet/pkg/config"
	. "github.com/geaaru/luet/pkg/logger"
	luet_pkg "github.com/geaaru/luet/pkg/package"
	luet_tree "github.com/geaaru/luet/pkg/tree"
)

type Stage4Worker struct {
	Levels  *Stage4Levels
	Map     map[string]*luet_pkg.DefaultPackage
	Changed map[string]*luet_pkg.DefaultPackage
}

func (pc *PortageConverter) Stage4() error {

	InfoC(GetAurora().Bold("Stage4 Starting..."))

	if len(pc.Solutions) == 0 {
		InfoC(GetAurora().Bold("Stage4: No solutions to elaborate. Nothing to do."))
		return nil
	}

	pc.ReciperBuild = luet_tree.NewCompilerRecipe(luet_pkg.NewInMemoryDatabase(false))
	pc.ReciperRuntime = luet_tree.NewInstallerRecipe(luet_pkg.NewInMemoryDatabase(false))

	err := pc.LoadTrees(pc.TreePaths)
	if err != nil {
		return err
	}

	// Create stage4 stuff
	var levels *Stage4Levels = nil
	worker := &Stage4Worker{
		Map:     make(map[string]*luet_pkg.DefaultPackage, 0),
		Changed: make(map[string]*luet_pkg.DefaultPackage, 0),
	}

	nSolutions := len(pc.Solutions)

	for idx, pkg := range pc.Solutions {

		if pkg.Upgrade && !pc.Override {
			continue
		}

		levels = NewStage4LevelsWithSize(1)
		worker.Levels = levels

		pack := pkg.ToPack(true)
		levels.Name = fmt.Sprintf(":microscope: [%s/%s-%s]",
			pack.GetCategory(), pack.GetName(), pack.GetVersion())

		// Check buildtime requires
		InfoC(GetAurora().Bold(fmt.Sprintf(":dna: [%s/%s-%s] [%d/%d]",
			pack.GetCategory(), pack.GetName(), pack.GetVersion(),
			idx+1, nSolutions)),
			"Preparing stage4 levels struct...")

		luetPkg := &luet_pkg.DefaultPackage{
			Name:     pack.GetName(),
			Category: pack.GetCategory(),
			Version:  pack.GetVersion(),
		}

		err := pc.Stage4AddDeps2Levels(luetPkg, nil, worker, 1, []string{})
		if err != nil {
			Warning(fmt.Sprintf(
				"Stage4: [%s/%s] Error on add deps to levels. Skip package - %s",
				pack.GetCategory(), pack.GetName(), err.Error()),
			)
			continue
		}
		// Setup level1 with all packages
		err = pc.Stage4AlignLevel1(worker)
		if err != nil {
			Warning(fmt.Sprintf(
				"Stage4: [%s/%s] Error on align levels. Skip package - %s",
				pack.GetCategory(), pack.GetName(), err.Error()),
			)
			continue
		}

		DebugC(fmt.Sprintf(
			"Stage4: Created levels structs of %d trees for %d packages.",
			len(levels.Levels), len(levels.Map)))

		pc.Stage4LevelsDumpWrapper(levels, "Starting structure")

		err = levels.Resolve()
		if err != nil {
			Warning(fmt.Sprintf(
				"Stage4: [%s/%s] Error on resolve. Skip package - %s",
				pack.GetCategory(), pack.GetName(), err.Error()),
			)
			continue
		}

		InfoC(
			fmt.Sprintf(
				":party_popper: [%s/%s-%s] Analysis completed (%d changes).",
				pack.GetCategory(), pack.GetName(), pack.GetVersion(),
				len(levels.Changed)))

		pc.Stage4LevelsDumpWrapper(levels, "Resolved structure")

		pc.stage4SyncChanges(worker)
	}

	err = pc.stage4UpdateBuildFiles(worker)
	if err != nil {
		return errors.New("Error on update build.yaml files: " + err.Error())
	}

	InfoC(GetAurora().Bold(
		fmt.Sprintf("Stage4 Completed. Packages updates: %d.", len(worker.Changed))))

	return nil
}

func (pc *PortageConverter) stage4UpdateBuildFiles(worker *Stage4Worker) error {

	if len(worker.Changed) == 0 {
		return nil
	}

	for _, pkg := range worker.Changed {

		ppp, err := pc.ReciperBuild.GetDatabase().FindPackages(pkg)
		if err != nil {
			return errors.New(
				fmt.Sprintf("Error on retrieve data of the package %s/%s: %s",
					pkg.GetCategory(), pkg.GetName(), err,
				))
		}

		buildFile := filepath.Join(ppp[0].GetPath(), "build.yaml")
		// Load Build Template file
		buildPack, err := NewLuetCompilationSpecSanitizedFromFile(buildFile)
		if err != nil {
			return err
		}

		prevReqs := len(buildPack.GetRequires())

		// Prepare requires
		reqs := []*luet_pkg.DefaultPackage{}

		for _, dep := range pkg.GetRequires() {
			reqs = append(reqs, &luet_pkg.DefaultPackage{
				Category: dep.GetCategory(),
				Name:     dep.GetName(),
				Version:  ">=0",
			})
		}

		buildPack.Requires(reqs)

		err = buildPack.WriteBuildDefinition(buildFile)
		if err != nil {
			return err
		}

		DebugC(fmt.Sprintf("[%s/%s] Update requires (%d -> %d).",
			pkg.GetCategory(), pkg.GetName(),
			prevReqs, len(reqs)))

	}

	return nil
}

func (pc *PortageConverter) Stage4LevelsDumpWrapper(levels *Stage4Levels, msg string) {
	if len(levels.Levels) > 10 {
		DebugC(fmt.Sprintf(
			"Stage4: %s:\n", msg))
		for idx, _ := range levels.Levels {
			DebugC(levels.Levels[idx].Dump())
		}

	} else {
		DebugC(fmt.Sprintf(
			"Stage4: %s:\n%s\n", msg, levels.Dump(),
		))
	}
}

func (pc *PortageConverter) Stage4AddDeps2Levels(pkg *luet_pkg.DefaultPackage,
	father *luet_pkg.DefaultPackage,
	w *Stage4Worker, level int, stack []string) error {

	key := fmt.Sprintf("%s/%s", pkg.GetCategory(), pkg.GetName())

	if cfg.LuetCfg.GetLogging().Level == "debug" {
		DebugC(fmt.Sprintf(
			"Adding pkg %s to level %d...",
			pkg.HumanReadableString(),
			level,
		))
	}

	if IsInStack(stack, key) {
		Error(fmt.Sprintf("For package %s found cycle: %v", key, stack))
		return errors.New("Cycle for package " + key)
	}
	stack = append(stack, key)

	// Check if level is already available
	if len(w.Levels.Levels) < level {
		tree := NewStage4Tree(level)
		w.Levels.AddTree(tree)
	}

	v, ok := w.Map[key]
	if ok {
		// Package already in map. I will use the same reference.
		pkg = v
	} else {

		pkg_search := &luet_pkg.DefaultPackage{
			Category: pkg.GetCategory(),
			Name:     pkg.GetName(),
			Version:  ">=0",
		}

		ppp, err := pc.ReciperBuild.GetDatabase().FindPackages(pkg_search)
		if err != nil {
			return errors.New(
				fmt.Sprintf(
					"Error on retrieve dependencies of the package %s/%s: %s",
					pkg.GetCategory(), pkg.GetName(), err.Error()))
		}

		if len(ppp) > 0 {
			pkg.Requires(ppp[0].GetRequires())
		} else {
			DebugC(fmt.Sprintf("No packages found on reciper for %s.", key))
		}

		w.Map[key] = pkg

	}

	// Add package to first level
	w.Levels.AddDependency(pkg, father, level-1)

	if len(pkg.GetRequires()) > 0 {

		// Add requires
		for _, dep := range pkg.GetRequires() {
			err := pc.Stage4AddDeps2Levels(dep, pkg, w, level+1, stack)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (pc *PortageConverter) stage4SyncChanges(w *Stage4Worker) {
	for idx, _ := range w.Levels.Changed {
		if _, ok := w.Changed[idx]; !ok {
			w.Changed[idx] = w.Levels.Changed[idx]
		}
	}
}

func (pc *PortageConverter) Stage4AlignLevel1(w *Stage4Worker) error {

	for pkg, v := range w.Levels.Map {

		if _, ok := w.Levels.Levels[0].Map[pkg]; !ok {

			DebugC(fmt.Sprintf("Adding package %s..", pkg))
			_, err := w.Levels.AddDependencyRecursive(v, nil, []string{}, 0)
			if err != nil {
				return err
			}
		}

	}

	return nil
}
