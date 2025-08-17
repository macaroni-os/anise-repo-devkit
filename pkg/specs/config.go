/*
Copyright © 2020-2025 Macaroni OS Linux
See AUTHORS and LICENSE for the license details and contributors.
*/
package specs

import (
	"errors"
	"fmt"
	"io/ioutil"

	"github.com/macaroni-os/anise-repo-devkit/pkg/version"

	. "github.com/geaaru/luet/pkg/logger"
	anise_pkg "github.com/geaaru/luet/pkg/package"
	"gopkg.in/yaml.v2"
)

func NewAniseRDConfig() *AniseRDConfig {
	return &AniseRDConfig{
		Cleaner: AniseRDCCleaner{
			Excludes: []string{},
		},
	}
}

func (c *AniseRDConfig) GetCleaner() *AniseRDCCleaner { return &c.Cleaner }
func (c *AniseRDConfig) GetList() *AniseRDCList       { return &c.List }

func (c *AniseRDCCleaner) HasExcludes() bool {
	return len(c.Excludes) > 0
}

func (c *AniseRDCList) HasFilters() bool {
	return len(c.ExcludePkgs) > 0
}

func (c *AnisePackage) GetName() string     { return c.Name }
func (c *AnisePackage) GetCategory() string { return c.Category }
func (c *AnisePackage) GetVersion() string  { return c.Version }
func (p *AnisePackage) HumanReadableString() string {
	return fmt.Sprintf("%s/%s-%s", p.Category, p.Name, p.Version)
}

func (c *AniseRDCList) ToIgnore(pkg *anise_pkg.DefaultPackage) bool {
	ans := false

	if c.HasFilters() {

		pSelector, err := version.ParseVersion(pkg.GetVersion())
		if err != nil {
			Warning(fmt.Sprintf(
				"Error on create package selector for package %s: %s",
				pkg.HumanReadableString(), err.Error()))
			return true
		}

		for _, f := range c.ExcludePkgs {
			if f.GetName() != pkg.GetName() ||
				f.GetCategory() != pkg.GetCategory() {
				continue
			}

			selector, err := version.ParseVersion(f.GetVersion())
			if err != nil {
				Warning(fmt.Sprintf(
					"Error on create version selector for package %s: %s",
					f.HumanReadableString(), err.Error()))
				continue
			}

			admit, err := version.PackageAdmit(selector, pSelector)
			if err != nil {
				Warning(fmt.Sprintf("Error on check package %s: %s",
					f.HumanReadableString(), err.Error()))
				continue
			}

			if admit {
				ans = true
			}

		}
	}

	return ans
}

func SpecsFromYaml(data []byte) (*AniseRDConfig, error) {
	ans := NewAniseRDConfig()
	if err := yaml.Unmarshal(data, ans); err != nil {
		return nil, err
	}
	return ans, nil
}

func LoadSpecsFile(file string) (*AniseRDConfig, error) {
	if file == "" {
		return nil, errors.New("Invalid file path")
	}

	content, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}

	ans, err := SpecsFromYaml(content)
	if err != nil {
		return nil, err
	}

	return ans, nil
}
