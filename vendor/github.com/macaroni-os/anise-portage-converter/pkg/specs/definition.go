/*
Copyright © 2021-2023 Macaroni OS Linux
See AUTHORS and LICENSE for the license details and contributors.
*/
package specs

import (
	"errors"
	"fmt"
	"io/ioutil"
	"path"
	"path/filepath"

	luet_pkg "github.com/geaaru/luet/pkg/package"
	gentoo "github.com/geaaru/pkgs-checker/pkg/gentoo"

	"gopkg.in/yaml.v2"
)

type PortageConverterSpecs struct {
	File string `json:"-" yaml:"-"`

	SkippedResolutions PortageConverterSkips `json:"skipped_resolutions,omitempty" yaml:"skipped_resolutions,omitempty"`

	TreePaths []string `json:"trees,omitempty" yaml:"trees,omitempty"`

	IncludeFiles         []string                   `json:"include_files,omitempty" yaml:"include_files,omitempty"`
	Artefacts            []PortageConverterArtefact `json:"artefacts,omitempty" yaml:"artefacts,omitempty"`
	BuildTmplFile        string                     `json:"build_template_file" yaml:"build_template_file"`
	BuildPortageTmplFile string                     `json:"build_portage_template_file,omitempty" yaml:"build_portage_template_file,omitempty"`

	// Reposcan options
	ReposcanRequiresWithSlot  bool                                `json:"reposcan_requires_slot,omitempty" yaml:"reposcan_requires_slot,omitempty"`
	ReposcanAllowEmpyKeywords bool                                `json:"reposcan_allow_empty_keywords,omitempty" yaml:"reposcan_allow_empty_keywords,omitempty"`
	ReposcanSources           []string                            `json:"reposcan_sources,omitempty" yaml:"reposcan_sources,omitempty"`
	ReposcanConstraints       PortageConverterReposcanConstraints `json:"reposcan_contraints,omitempty" yaml:"reposcan_contraints,omitempty"`
	ReposcanDisabledUseFlags  []string                            `json:"reposcan_disabled_use_flags,omitempty" yaml:"reposcan_disabled_use_flags,omitempty"`
	ReposcanDisabledKeywords  []string                            `json:"reposcan_disabled_keywords,omitempty" yaml:"reposcan_disabled_keywords,omitempty"`

	Replacements PortageConverterReplacements `json:"replacements,omitempty" yaml:"replacements,omitempty"`

	BuildLayers   []PortageConverterBuildLayer `json:"build_layers,omitempty" yaml:"build_layers,omitempty"`
	IncludeLayers []string                     `json:"include_layers,omitempty" yaml:"include_layers,omitempty"`

	Annotations map[string]interface{} `json:"global_annotations,omitempty" yaml:"global_annotations,omitempty"`

	MapArtefacts             map[string]*PortageConverterArtefact       `json:"-" yaml:"-"`
	MapReplacementsRuntime   map[string]*PortageConverterReplacePackage `json:"-" yaml:"-"`
	MapReplacementsBuildtime map[string]*PortageConverterReplacePackage `json:"-" yaml:"-"`
	MapBuildLayer            map[string]*PortageConverterBuildLayer     `json:"-" yaml:"-"`
}

type PortageConverterBuildLayer struct {
	Layer    PortageConverterPkg `json:"layer,omitempty" yaml:"layer,omitempty"`
	Packages []string            `json:"packages" yaml:"packages"`
}

type PortageConverterReposcanConstraints struct {
	Packages []string `json:"packages,omitempty" yaml:"packages,omitempty"`
}

type PortageConverterSkips struct {
	Packages        []PortageConverterPkg `json:"packages,omitempty" yaml:"packages,omitempty"`
	Categories      []string              `json:"categories,omitempty" yaml:"categories,omitempty"`
	BuildCategories []string              `json:"build_categories,omitempty" yaml:"build_categories,omitempty"`
}

type PortageConverterPkg struct {
	Name     string `json:"name" yaml:"name"`
	Category string `json:"category" yaml:"category"`
	Version  string `json:"version" yaml:"version"`
}

type PortageConverterArtefact struct {
	Tree            string                   `json:"tree" yaml:"tree"`
	Uses            PortageConverterUseFlags `json:"uses,omitempty" yaml:"uses,omitempty"`
	IgnoreBuildDeps bool                     `json:"ignore_build_deps,omitempty" yaml:"ignore_build_deps,omitempty"`
	Packages        []string                 `json:"packages" yaml:"packages"`
	OverrideVersion string                   `json:"override_version,omitempty" yaml:"override_version,omitempty"`

	CustomPath string   `json:"custom_path,omitempty" yaml:"custom_path,omitempty"`
	Conditions []string `json:"conditions,omitempty" yaml:"conditions,omitempty"`

	Replacements PortageConverterReplacements `json:"replacements,omitempty" yaml:"replacements,omitempty"`
	Mutations    PortageConverterMutations    `json:"mutations,omitempty" yaml:"mutations,omitempty"`

	Finalize    Finalizer              `json:"finalizer,omitempty" yaml:"finalizer,omitempty"`
	Annotations map[string]interface{} `json:"annotations,omitempty" yaml:"annotations,omitempty"`

	MapReplacementsRuntime   map[string]*PortageConverterReplacePackage `json:"-" yaml:"-"`
	MapReplacementsBuildtime map[string]*PortageConverterReplacePackage `json:"-" yaml:"-"`
	MapIgnoreRuntime         map[string]bool                            `json:"-" yaml:"-"`
	MapIgnoreBuildtime       map[string]bool                            `json:"-" yaml:"-"`
}

type Finalizer struct {
	Shell     []string `json:"shell,omitempty" yaml:"shell,omitempty"`
	Install   []string `json:"install,omitempty" yaml:"install,omitempty"`
	Uninstall []string `json:"uninstall,omitempty" yaml:"uninstall,omitempty"`
}

type PortageConverterMutations struct {
	RuntimeDeps   PortageConverterMutationDeps `json:"runtime_deps,omitempty" yaml:"runtime_deps,omitempty"`
	BuildTimeDeps PortageConverterMutationDeps `json:"buildtime_deps,omitempty" yaml:"buildtime_deps,omitempty"`
	Uses          []string                     `json:"uses,omitempty" yaml:"uses,omitempty"`
	Provides      []PortageConverterPkg        `json:"provides,omitempty" yaml:"provides,omitempty"`
	Conflicts     []PortageConverterPkg        `json:"conflicts,omitempty" yaml:"conflicts,omitempty"`
}

type PortageConverterMutationDeps struct {
	Packages []PortageConverterPkg `json:"packages,omitempty" yaml:"packages,omitempty"`
}

type PortageConverterReplacements struct {
	RuntimeDeps  PortageConverterDepReplacements `json:"runtime_deps,omitempty" yaml:"runtime_deps,omitempty"`
	BuiltimeDeps PortageConverterDepReplacements `json:"buildtime_deps,omitempty" yaml:"buildtime_deps,omitempty"`
}

type PortageConverterDepReplacements struct {
	Packages []PortageConverterReplacePackage `json:"packages,omitempty" yaml:"packages,omitempty"`
	Ignore   []PortageConverterPkg            `json:"ignore,omitempty" yaml:"ignore,omitempty"`
}

type PortageConverterReplacePackage struct {
	From PortageConverterPkg `json:"from" yaml:"from"`
	To   PortageConverterPkg `json:"to" yaml:"to"`
}

type PortageConverterUseFlags struct {
	Disabled []string `json:"disabled,omitempty" yaml:"disabled,omitempty"`
	Enabled  []string `json:"enabled,omitempty" yaml:"enabled,omitempty"`
}

type PortageConverterInclude struct {
	SkippedResolutions PortageConverterSkips        `json:"skipped_resolutions,omitempty" yaml:"skipped_resolutions,omitempty"`
	Artefacts          []PortageConverterArtefact   `json:"artefacts,omitempty" yaml:"artefacts,omitempty"`
	BuildLayers        []PortageConverterBuildLayer `json:"build_layers,omitempty" yaml:"build_layers,omitempty"`
}

func SpecsFromYaml(data []byte) (*PortageConverterSpecs, error) {
	ans := &PortageConverterSpecs{}
	if err := yaml.Unmarshal(data, ans); err != nil {
		return nil, err
	}
	return ans, nil
}

func IncludeFromYaml(data []byte) (*PortageConverterInclude, error) {
	ans := &PortageConverterInclude{}
	if err := yaml.Unmarshal(data, ans); err != nil {
		return nil, err
	}
	return ans, nil
}

func IncludeLayerFromYaml(data []byte) (*PortageConverterBuildLayer, error) {
	ans := &PortageConverterBuildLayer{}
	if err := yaml.Unmarshal(data, ans); err != nil {
		return nil, err
	}
	return ans, nil
}

func LoadSpecsFile(file string) (*PortageConverterSpecs, error) {

	if file == "" {
		return nil, errors.New("Invalid file path")
	}

	content, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Error on read file %s: %s",
			file, err.Error()))
	}

	ans, err := SpecsFromYaml(content)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Error on parse file %s: %s",
			file, err.Error()))
	}

	ans.File = file

	absPath, err := filepath.Abs(path.Dir(file))
	if err != nil {
		return nil, err
	}

	if len(ans.IncludeFiles) > 0 {

		for _, include := range ans.IncludeFiles {

			if include[0:1] != "/" {
				include = filepath.Join(absPath, include)
			}
			content, err := ioutil.ReadFile(include)
			if err != nil {
				return nil, errors.New(fmt.Sprintf("Error on read file %s: %s",
					include, err.Error()))
			}

			data, err := IncludeFromYaml(content)
			if err != nil {
				return nil, errors.New(fmt.Sprintf("Error on parse file %s: %s",
					include, err.Error()))
			}

			if len(data.SkippedResolutions.Packages) > 0 {
				ans.SkippedResolutions.Packages = append(ans.SkippedResolutions.Packages,
					data.SkippedResolutions.Packages...)
			}

			if len(data.SkippedResolutions.Categories) > 0 {
				ans.SkippedResolutions.Categories = append(ans.SkippedResolutions.Categories,
					data.SkippedResolutions.Categories...)
			}

			if len(data.SkippedResolutions.BuildCategories) > 0 {
				ans.SkippedResolutions.BuildCategories = append(ans.SkippedResolutions.BuildCategories,
					data.SkippedResolutions.BuildCategories...)
			}

			if len(data.Artefacts) > 0 {
				ans.Artefacts = append(ans.Artefacts, data.Artefacts...)
			}

			if len(data.BuildLayers) > 0 {
				ans.BuildLayers = append(ans.BuildLayers, data.BuildLayers...)
			}
		}
	}

	if ans.BuildTmplFile != "" && ans.BuildTmplFile[0:1] != "/" {
		// Convert in abs path
		ans.BuildTmplFile = filepath.Join(absPath, ans.BuildTmplFile)
	}

	if ans.BuildPortageTmplFile != "" && ans.BuildPortageTmplFile[0:1] != "/" {
		// Convert in abs path
		ans.BuildPortageTmplFile = filepath.Join(absPath, ans.BuildPortageTmplFile)
	}

	if len(ans.IncludeLayers) > 0 {

		for _, include := range ans.IncludeLayers {

			if include[0:1] != "/" {
				include = filepath.Join(absPath, include)
			}

			content, err := ioutil.ReadFile(include)
			if err != nil {
				return nil, errors.New(fmt.Sprintf("Error on read file %s: %s",
					include, err.Error()))
			}

			data, err := IncludeLayerFromYaml(content)
			if err != nil {
				return nil, errors.New(fmt.Sprintf("Error on parse file %s: %s",
					include, err.Error()))
			}

			if data.Layer.Category == "" || data.Layer.Name == "" || len(data.Packages) == 0 {
				return nil, errors.New(fmt.Sprintf("Invalid layer file %s.",
					include))
			}

			ans.BuildLayers = append(ans.BuildLayers, *data)
		}

	}

	return ans, nil
}

func (s *PortageConverterSpecs) GenerateArtefactsMap() {

	s.MapArtefacts = make(map[string]*PortageConverterArtefact, 0)

	for idx, _ := range s.Artefacts {
		for _, pkg := range s.Artefacts[idx].Packages {
			k := pkg
			gp, err := gentoo.ParsePackageStr(pkg)
			if err == nil && gp.Slot == "0" {
				// Ensure to skipt slot 0 on key.
				k = fmt.Sprintf("%s/%s", gp.Category, gp.Name)
			}

			s.MapArtefacts[k] = &s.Artefacts[idx]
		}
	}
}

func (s *PortageConverterSpecs) HasRuntimeReplacement(pkg string) bool {
	_, ans := s.MapReplacementsRuntime[pkg]
	return ans
}

func (s *PortageConverterSpecs) HasBuildtimeReplacement(pkg string) bool {
	_, ans := s.MapReplacementsBuildtime[pkg]
	return ans
}

func (s *PortageConverterSpecs) GetBuildtimeReplacement(pkg string) (*PortageConverterReplacePackage, error) {
	ans, ok := s.MapReplacementsBuildtime[pkg]
	if ok {
		return ans, nil
	}
	return ans, errors.New("No replacement found for key " + pkg)
}

func (s *PortageConverterSpecs) GetRuntimeReplacement(pkg string) (*PortageConverterReplacePackage, error) {
	ans, ok := s.MapReplacementsRuntime[pkg]
	if ok {
		return ans, nil
	}
	return ans, errors.New("No replacement found for key " + pkg)
}

func (s *PortageConverterSpecs) HasBuildLayer(pkg string) bool {
	gp, err := gentoo.ParsePackageStr(pkg)
	k := pkg
	if err == nil && gp.Slot == "0" {
		// Ensure to skipt slot 0 on key.
		k = fmt.Sprintf("%s/%s", gp.Category, gp.Name)
	}
	_, ans := s.MapBuildLayer[k]
	return ans
}

func (s *PortageConverterSpecs) GetBuildLayer(pkg string) (*PortageConverterBuildLayer, error) {
	ans, ok := s.MapBuildLayer[pkg]
	if ok {
		return ans, nil
	}
	return ans, errors.New("no build layer found for key " + pkg)
}

func (s *PortageConverterSpecs) GenerateBuildLayerMap() {
	s.MapBuildLayer = make(map[string]*PortageConverterBuildLayer, 0)

	if len(s.BuildLayers) > 0 {
		for idx, _ := range s.BuildLayers {
			for _, pkg := range s.BuildLayers[idx].Packages {
				gp, err := gentoo.ParsePackageStr(pkg)
				k := pkg
				if err == nil && gp.Slot == "0" {
					// Ensure to skipt slot 0 on key.
					k = fmt.Sprintf("%s/%s", gp.Category, gp.Name)
				}
				s.MapBuildLayer[k] = &s.BuildLayers[idx]
			}
		}
	}
}

func (s *PortageConverterSpecs) PackageIsALayer(pkg *gentoo.GentooPackage) bool {
	ans := false

	if len(s.BuildLayers) > 0 {
		for idx, _ := range s.BuildLayers {
			ans, _ := s.BuildLayers[idx].Layer.EqualTo(pkg)
			if ans {
				break
			}
		}
	}

	return ans
}

func (s *PortageConverterSpecs) GenerateReplacementsMap() {
	s.MapReplacementsRuntime = make(map[string]*PortageConverterReplacePackage, 0)
	s.MapReplacementsBuildtime = make(map[string]*PortageConverterReplacePackage, 0)

	// Add key from global map
	if len(s.Replacements.RuntimeDeps.Packages) > 0 {
		for ridx, r := range s.Replacements.RuntimeDeps.Packages {
			s.MapReplacementsRuntime[fmt.Sprintf("%s/%s", r.From.Category, r.From.Name)] =
				&s.Replacements.RuntimeDeps.Packages[ridx]
		}
	}

	if len(s.Replacements.BuiltimeDeps.Packages) > 0 {
		for ridx, r := range s.Replacements.BuiltimeDeps.Packages {
			s.MapReplacementsBuildtime[fmt.Sprintf("%s/%s", r.From.Category, r.From.Name)] =
				&s.Replacements.BuiltimeDeps.Packages[ridx]
		}
	}

	// Create artefact maps
	for idx, _ := range s.Artefacts {
		s.Artefacts[idx].MapReplacementsBuildtime = make(map[string]*PortageConverterReplacePackage, 0)
		s.Artefacts[idx].MapReplacementsRuntime = make(map[string]*PortageConverterReplacePackage, 0)
		s.Artefacts[idx].MapIgnoreBuildtime = make(map[string]bool, 0)
		s.Artefacts[idx].MapIgnoreRuntime = make(map[string]bool, 0)

		if len(s.Artefacts[idx].Replacements.RuntimeDeps.Packages) > 0 {
			for ridx, r := range s.Artefacts[idx].Replacements.RuntimeDeps.Packages {
				s.Artefacts[idx].MapReplacementsRuntime[fmt.Sprintf(
					"%s/%s", r.From.Category, r.From.Name)] =
					&s.Artefacts[idx].Replacements.RuntimeDeps.Packages[ridx]
			}
		}

		if len(s.Artefacts[idx].Replacements.RuntimeDeps.Ignore) > 0 {
			for _, r := range s.Artefacts[idx].Replacements.RuntimeDeps.Ignore {
				s.Artefacts[idx].MapIgnoreRuntime[fmt.Sprintf(
					"%s/%s", r.Category, r.Name)] = true
			}
		}

		if len(s.Artefacts[idx].Replacements.BuiltimeDeps.Packages) > 0 {
			for ridx, r := range s.Artefacts[idx].Replacements.BuiltimeDeps.Packages {
				s.Artefacts[idx].MapReplacementsBuildtime[fmt.Sprintf(
					"%s/%s", r.From.Category, r.From.Name)] =
					&s.Artefacts[idx].Replacements.BuiltimeDeps.Packages[ridx]
			}
		}

		if len(s.Artefacts[idx].Replacements.BuiltimeDeps.Ignore) > 0 {
			for _, r := range s.Artefacts[idx].Replacements.BuiltimeDeps.Ignore {
				s.Artefacts[idx].MapIgnoreBuildtime[fmt.Sprintf(
					"%s/%s", r.Category, r.Name)] = true
			}
		}

	}
}

func (s *PortageConverterSpecs) GetArtefactByPackage(pkg string) (*PortageConverterArtefact, error) {
	if a, ok := s.MapArtefacts[pkg]; ok {
		return a, nil
	}
	return nil, errors.New("Package not found")
}

func (s *PortageConverterSpecs) GetArtefacts() []PortageConverterArtefact {
	return s.Artefacts
}

func (s *PortageConverterSpecs) AddReposcanSource(source string) {
	s.ReposcanSources = append(s.ReposcanSources, source)
}

func (s *PortageConverterSpecs) AddReposcanDisabledUseFlags(uses []string) {
	s.ReposcanDisabledUseFlags = append(s.ReposcanDisabledUseFlags, uses...)
}

func (s *PortageConverterSpecs) GetGlobalAnnotations() *map[string]interface{} {
	return &s.Annotations
}

type PortageResolver interface {
	Resolve(pkg string, opts *PortageResolverOpts) (*PortageSolution, error)
}

type PortageResolverOpts struct {
	EnableUseFlags   []string
	DisabledUseFlags []string
	Conditions       []string
}

type PortageSolution struct {
	Package          gentoo.GentooPackage   `json:"package"`
	PackageDir       string                 `json:"package_dir"`
	BuildDeps        []gentoo.GentooPackage `json:"build-deps,omitempty"`
	RuntimeDeps      []gentoo.GentooPackage `json:"runtime-deps,omitempty"`
	RuntimeConflicts []gentoo.GentooPackage `json:"runtime-conflicts,omitempty"`
	BuildConflicts   []gentoo.GentooPackage `json:"build-conflicts,omitempty"`

	Description string                 `json:"description,omitempty"`
	Uri         []string               `json:"uri,omitempty"`
	Labels      map[string]string      `json:"labels,omitempty"`
	Annotations map[string]interface{} `json:"annotations,omitempty" yaml:"annotations,omitempty"`

	OverrideVersion string                   `json:"override_version,omitempty"`
	Upgrade         bool                     `json:"upgrade,omitempty"`
	PackageUpgraded *luet_pkg.DefaultPackage `json:"-",omitempty"`
}
