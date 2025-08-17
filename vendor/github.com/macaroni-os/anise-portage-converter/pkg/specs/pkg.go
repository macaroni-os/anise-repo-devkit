/*
Copyright © 2021-2023 Macaroni OS Linux
See AUTHORS and LICENSE for the license details and contributors.
*/
package specs

import (
	"errors"
	"fmt"

	gentoo "github.com/geaaru/pkgs-checker/pkg/gentoo"
)

func (p *PortageConverterPkg) GetPackageName() string {
	return fmt.Sprintf("%s/%s", p.Category, p.Name)
}

func (p *PortageConverterPkg) EqualTo(pkg *gentoo.GentooPackage) (bool, error) {
	ans := false

	if pkg == nil || pkg.Category == "" || pkg.Name == "" {
		return false, errors.New("Invalid package for EqualTo")
	}

	if p.GetPackageName() == pkg.GetPackageName() {
		ans = true
	}

	return ans, nil
}
