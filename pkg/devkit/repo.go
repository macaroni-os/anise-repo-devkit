/*
Copyright © 2020-2025 Macaroni OS Linux
See AUTHORS and LICENSE for the license details and contributors.
*/

package devkit

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/macaroni-os/anise-repo-devkit/pkg/backends"
	specs "github.com/macaroni-os/anise-repo-devkit/pkg/specs"

	. "github.com/geaaru/luet/pkg/logger"
	anise_pkg "github.com/geaaru/luet/pkg/package"
	anise_tree "github.com/geaaru/luet/pkg/tree"
	artifact "github.com/geaaru/luet/pkg/v2/compiler/types/artifact"
	tmtools "github.com/geaaru/time-master/pkg/tools"
)

type RepoKnife struct {
	Specs          *specs.AniseRDConfig
	BackendHandler specs.RepoBackendHandler
	ReciperRuntime anise_tree.Builder

	PkgsMap        map[string]string
	MetaMap        map[string]*artifact.PackageArtifact
	Files2Remove   []string
	Verbose        bool
	ProcessedFiles int
}

func NewRepoKnife(s *specs.AniseRDConfig,
	backend, path string, opts map[string]string) (*RepoKnife, error) {
	var err error
	var handler specs.RepoBackendHandler

	ans := &RepoKnife{
		Specs:          s,
		ReciperRuntime: anise_tree.NewInstallerRecipe(anise_pkg.NewInMemoryDatabase(false)),
		PkgsMap:        make(map[string]string, 0),
		MetaMap:        make(map[string]*artifact.PackageArtifact, 0),
	}

	switch backend {
	case "local":
		handler, err = backends.NewBackendLocal(s, path)
	case "mottainai":
		handler, err = backends.NewBackendMottainai(s, path, opts)
	case "minio":
		handler, err = backends.NewBackendMinio(s, path, opts)
	default:
		return nil, errors.New("Invalid backend")
	}

	if err != nil {
		return nil, err
	}
	ans.BackendHandler = handler
	return ans, nil
}

func (c *RepoKnife) LoadTrees(treePath []string) error {

	// Load trees
	for _, t := range treePath {
		if c.Verbose {
			InfoC(fmt.Sprintf(":evergreen_tree: Loading tree %s...", t))
		} else {
			DebugC(fmt.Sprintf(":evergreen_tree: Loading tree %s...", t))
		}
		err := c.ReciperRuntime.Load(t)
		if err != nil {
			return errors.New("Error on load tree" + err.Error())
		}
	}

	return nil
}

func (c *RepoKnife) Analyze() error {

	// Reset previous values
	c.PkgsMap = make(map[string]string, 0)
	c.MetaMap = make(map[string]*artifact.PackageArtifact, 0)
	c.Files2Remove = []string{}

	// Retrieve the list of the files
	files, err := c.BackendHandler.GetFilesList()
	if err != nil {
		return err
	}
	c.ProcessedFiles = len(files)

	if c.Specs.GetCleaner().HasExcludes() {
		files, err = c.GetFilteredList(files)
		if err != nil {
			return err
		}
	}

	// Exclude repository files
	repoRegex := []string{
		"repository.meta.yaml.tar.*|repository.meta.yaml",
		"repository.yaml",
		"tree.tar.*|tree.tar",
		"compilertree.tar.*|compilertree.tar",
	}

	metaFilesRegex := []string{
		".*metadata.yaml",
	}

	pkgFilesRegex := []string{
		".*package.tar|.*package.tar.*",
	}

	for _, f := range files {
		if tmtools.RegexEntry(f, repoRegex) {
			DebugC(fmt.Sprintf("Ignoring repository file %s", f))
			continue
		}

		if c.Verbose {
			InfoC(fmt.Sprintf("[%s] Analyzing...", f))
		} else {
			DebugC(fmt.Sprintf("[%s] Analyzing...", f))
		}

		if tmtools.RegexEntry(f, metaFilesRegex) {

			art, err := c.BackendHandler.GetMetadata(f)
			if err != nil {
				return err
			}

			c.MetaMap[f] = art
		} else if tmtools.RegexEntry(f, pkgFilesRegex) {

			replaceRegex := regexp.MustCompile(
				`.package.tar$|.package.tar.gz$|.package.tar.zst$`,
			)

			metaFile := replaceRegex.ReplaceAllString(f, ".metadata.yaml")
			c.PkgsMap[f] = metaFile

		} else {
			// POST: file to remove
			c.Files2Remove = append(c.Files2Remove, f)
		}
	}

	// Check if there are all package for every metafile
	meta2Remove := []string{}
	for f, art := range c.MetaMap {
		pkg := filepath.Base(art.Path)

		if _, ok := c.PkgsMap[pkg]; !ok {
			if c.Verbose {
				InfoC(fmt.Sprintf(
					"No tarball found for metafile %s. I delete metafile.",
					f))
			} else {
				DebugC(fmt.Sprintf(
					"No tarball found for metafile %s. I delete metafile.",
					f))
			}
			c.Files2Remove = append(c.Files2Remove, f)
			meta2Remove = append(meta2Remove, f)
		}
	}

	for _, m := range meta2Remove {
		delete(c.PkgsMap, m)
	}

	// Check if there are all metadata for every package tarball
	for f, meta := range c.PkgsMap {
		if _, ok := c.MetaMap[meta]; !ok {
			if c.Verbose {
				InfoC(fmt.Sprintf(
					"No tarball file available for meta %s. I delete the tarball.",
					f))
			} else {
				DebugC(fmt.Sprintf(
					"No tarball file available for meta %s. I delete the tarball.",
					f))
			}
			c.Files2Remove = append(c.Files2Remove, f)
		}
	}

	//
	err = c.CheckFilesWithTrees()
	if err != nil {
		return err
	}

	return nil
}

func (c *RepoKnife) CheckFilesWithTrees() error {

	for m, art := range c.MetaMap {

		pkg := anise_pkg.NewPackage(
			art.CompileSpec.Package.Name,
			art.CompileSpec.Package.Version,
			[]*anise_pkg.DefaultPackage{},
			[]*anise_pkg.DefaultPackage{},
		)
		pkg.Category = art.CompileSpec.Package.Category

		p, _ := c.ReciperRuntime.GetDatabase().FindPackage(pkg)
		if p == nil {

			pkgFile := filepath.Base(art.Path)

			if c.Verbose {
				InfoC(fmt.Sprintf(
					"[%s] No more available in the repo. I will delete it.",
					pkg.HumanReadableString(),
				))
			} else {
				DebugC(fmt.Sprintf(
					"[%s] No more available in the repo. I will delete it.",
					pkg.HumanReadableString(),
				))
			}

			c.Files2Remove = append(c.Files2Remove, m)
			c.Files2Remove = append(c.Files2Remove, pkgFile)
		}

	}

	return nil
}

func (c *RepoKnife) GetFilteredList(files []string) ([]string, error) {
	ans := []string{}

	for _, f := range files {
		if !tmtools.RegexEntry(f, c.Specs.GetCleaner().Excludes) {
			ans = append(ans, f)
		}
	}

	return ans, nil
}
