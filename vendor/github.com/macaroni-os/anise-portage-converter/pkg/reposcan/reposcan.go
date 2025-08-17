/*
Copyright © 2021-2023 Macaroni OS Linux
See AUTHORS and LICENSE for the license details and contributors.
*/
package reposcan

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	gentoo "github.com/geaaru/pkgs-checker/pkg/gentoo"
	"github.com/macaroni-os/anise-portage-converter/pkg/specs"

	"gopkg.in/yaml.v2"
)

const (
	CacheDataVersion = "1.0.6"
)

type RepoScanSpec struct {
	CacheDataVersion string                  `json:"cache_data_version" yaml:"cache_data_version"`
	Atoms            map[string]RepoScanAtom `json:"atoms" yaml:"atoms"`
	MetadataErrors   map[string]RepoScanAtom `json:"metadata_errors,omitempty" yaml:"metadata_errors,omitempty"`

	File string `json:"-"`
}

type RepoScanAtom struct {
	Atom string `json:"atom,omitempty" yaml:"atom,omitempty"`

	Category string     `json:"category,omitempty" yaml:"category,omitempty"`
	Package  string     `json:"package,omitempty" yaml:"package,omitempty"`
	Revision string     `json:"revision,omitempty" yaml:"revision,omitempty"`
	CatPkg   string     `json:"catpkg,omitempty" yaml:"catpkg,omitempty"`
	Eclasses [][]string `json:"eclasses,omitempty" yaml:"eclasses,omitempty"`

	Kit    string `json:"kit,omitempty" yaml:"kit,omitempty"`
	Branch string `json:"branch,omitempty" yaml:"branch,omitempty"`

	// Relations contains the list of the keys defined on
	// relations_by_kind. The values could be RDEPEND, DEPEND, BDEPEND
	Relations       []string            `json:"relations,omitempty" yaml:"relations,omitempty"`
	RelationsByKind map[string][]string `json:"relations_by_kind,omitempty" yaml:"relations_by_kind,omitempty"`

	// Metadata contains ebuild variables.
	// Ex: SLOT, SRC_URI, HOMEPAGE, etc.
	Metadata    map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	MetadataOut string            `json:"metadata_out,omitempty" yaml:"metadata_out,omitempty"`

	ManifestMd5 string `json:"manifest_md5,omitempty" yaml:"manifest_md5,omitempty"`
	Md5         string `json:"md5,omitempty" yaml:"md5,omitempty"`

	// Fields present on failure
	Status string `json:"status,omitempty" yaml:"status,omitempty"`
	Output string `json:"output,omitempty" yaml:"output,omitempty"`

	Files []RepoScanFile `json:"files,omitempty" yaml:"files,omitempty"`
}

type RepoScanFile struct {
	SrcUri []string          `json:"src_uri"`
	Size   string            `json:"size"`
	Hashes map[string]string `json:"hashes"`
	Name   string            `json:"name"`
}

func (r *RepoScanSpec) Yaml() (string, error) {
	data, err := yaml.Marshal(r)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (r *RepoScanSpec) Json() (string, error) {
	data, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (r *RepoScanSpec) WriteJsonFile(f string) error {
	// TODO: Check if using writer from json marshal
	//       could be better
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}

	return os.WriteFile(f, data, 0644)
}

func (r *RepoScanAtom) GetPackageName() string {
	return fmt.Sprintf("%s/%s", r.GetCategory(), r.Package)
}

func (r *RepoScanAtom) GetCategory() string {
	slot := "0"

	if r.HasMetadataKey("SLOT") {
		slot = r.GetMetadataValue("SLOT")
		// We ignore subslot atm.
		if strings.Contains(slot, "/") {
			slot = slot[0:strings.Index(slot, "/")]
		}

	}

	return specs.SanitizeCategory(r.Category, slot)
}

func (r *RepoScanAtom) Yaml() (string, error) {
	data, err := yaml.Marshal(r)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (r *RepoScanAtom) Json() (string, error) {
	data, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (r *RepoScanAtom) HasMetadataKey(k string) bool {
	_, ans := r.Metadata[k]
	return ans
}

func (r *RepoScanAtom) GetMetadataValue(k string) string {
	ans, _ := r.Metadata[k]
	return ans
}

func (r *RepoScanAtom) ToGentooPackage() (*gentoo.GentooPackage, error) {
	ans, err := gentoo.ParsePackageStr(r.Atom)
	if err != nil {
		return nil, err
	}

	// Retrieve license
	if l, ok := r.Metadata["LICENSE"]; ok {
		ans.License = l
	}

	if slot, ok := r.Metadata["SLOT"]; ok {
		// TOSEE: We ignore subslot atm.
		if strings.Contains(slot, "/") {
			slot = slot[0:strings.Index(slot, "/")]
		}
		ans.Slot = slot
	}

	ans.Repository = r.Kit

	return ans, nil
}

func (r *RepoScanAtom) AddRelations(pkgname string) {
	isPresent := false
	for idx := range r.Relations {
		if r.Relations[idx] == pkgname {
			isPresent = true
			break
		}
	}

	if !isPresent {
		r.Relations = append(r.Relations, pkgname)
	}
}

func (r *RepoScanAtom) AddRelationsByKind(kind, pkgname string) {
	isPresent := false
	list, kindPresent := r.RelationsByKind[kind]

	if kindPresent {
		for idx := range list {
			if list[idx] == pkgname {
				isPresent = true
				break
			}
		}
	} else {
		r.RelationsByKind[kind] = []string{}
	}

	if !isPresent {
		r.RelationsByKind[kind] = append(r.RelationsByKind[kind], pkgname)
	}
}

func (r *RepoScanAtom) GetRuntimeDeps() ([]gentoo.GentooPackage, error) {
	ans := []gentoo.GentooPackage{}

	if len(r.Relations) > 0 {
		if _, ok := r.RelationsByKind["RDEPEND"]; ok {

			deps, err := r.getDepends("RDEPEND")
			if err != nil {
				return ans, err
			}
			ans = append(ans, deps...)
		}
		// TODO: Check if it's needed add PDEPEND here
	}

	return ans, nil
}

func (r *RepoScanAtom) GetBuildtimeDeps() ([]gentoo.GentooPackage, error) {
	ans := []gentoo.GentooPackage{}

	if len(r.Relations) > 0 {
		if _, ok := r.RelationsByKind["DEPEND"]; ok {
			deps, err := r.getDepends("DEPEND")
			if err != nil {
				return ans, err
			}
			ans = append(ans, deps...)
		}

		if _, ok := r.RelationsByKind["BDEPEND"]; ok {
			deps, err := r.getDepends("BDEPEND")
			if err != nil {
				return ans, err
			}
			ans = append(ans, deps...)
		}
	}

	return ans, nil
}

func (r *RepoScanAtom) getDepends(depType string) ([]gentoo.GentooPackage, error) {
	ans := []gentoo.GentooPackage{}
	if _, ok := r.RelationsByKind[depType]; ok {

		for _, pkg := range r.RelationsByKind[depType] {
			gp, err := gentoo.ParsePackageStr(pkg)
			if err != nil {
				return ans, err
			}
			gp.Slot = ""
			ans = append(ans, *gp)
		}
	}

	return ans, nil
}
