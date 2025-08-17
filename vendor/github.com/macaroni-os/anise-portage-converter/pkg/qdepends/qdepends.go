/*
Copyright © 2021-2023 Macaroni OS Linux
See AUTHORS and LICENSE for the license details and contributors.
*/
package qdepends

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/macaroni-os/anise-portage-converter/pkg/specs"

	helpers "github.com/MottainaiCI/lxd-compose/pkg/helpers"
	gentoo "github.com/geaaru/pkgs-checker/pkg/gentoo"
)

type QDependsResolver struct{}

func NewQDependsResolver() *QDependsResolver {
	return &QDependsResolver{}
}

func retrieveVersion(solution *specs.PortageSolution) error {
	var outBuffer, errBuffer bytes.Buffer

	cmd := []string{"qdepends", "-qC"}

	pkg := solution.Package.GetPackageName()
	if solution.Package.Slot != "0" {
		pkg = fmt.Sprintf("%s:%s", pkg, solution.Package.Slot)
	}
	cmd = append(cmd, pkg)

	qdepends := exec.Command(cmd[0], cmd[1:]...)
	qdepends.Stdout = helpers.NewNopCloseWriter(&outBuffer)
	qdepends.Stderr = helpers.NewNopCloseWriter(&errBuffer)

	err := qdepends.Start()
	if err != nil {
		return err
	}

	err = qdepends.Wait()
	if err != nil {
		return err
	}

	ans := qdepends.ProcessState.ExitCode()
	if ans != 0 {
		return errors.New("Error on running rdepends for package " + pkg + ": " + errBuffer.String())
	}

	out := outBuffer.String()
	if len(out) > 0 {

		words := strings.Split(out, " ")
		p := strings.TrimSuffix(words[0], "\n")

		gp, err := gentoo.ParsePackageStr(p[:len(p)-1])
		if err != nil {
			return errors.New("On convert pkg " + p + ": " + err.Error())
		}

		solution.Package.Version = gp.Version
		solution.Package.VersionSuffix = gp.VersionSuffix

	} else {
		return errors.New("No version found for package " + solution.Package.GetPackageName())
	}

	return nil
}

func SanitizeSlot(pkg *gentoo.GentooPackage) {
	if strings.Index(pkg.Slot, "/") > 0 {
		pkg.Slot = pkg.Slot[0:strings.Index(pkg.Slot, "/")]
	}

	if pkg.Slot == "*" {
		pkg.Slot = "0"
	}
}

func runQdepends(solution *specs.PortageSolution, runtime bool) error {
	var outBuffer, errBuffer bytes.Buffer

	cmd := []string{"qdepends", "-qC", "-F", "deps"}

	if runtime {
		cmd = append(cmd, "-r")
	} else {
		cmd = append(cmd, "-bd")
	}

	pkg := solution.Package.GetPackageName()
	if solution.Package.Slot != "0" {
		pkg = fmt.Sprintf("%s:%s", pkg, solution.Package.Slot)
	}
	cmd = append(cmd, pkg)

	qdepends := exec.Command(cmd[0], cmd[1:]...)
	qdepends.Stdout = helpers.NewNopCloseWriter(&outBuffer)
	qdepends.Stderr = helpers.NewNopCloseWriter(&errBuffer)

	err := qdepends.Start()
	if err != nil {
		return err
	}

	err = qdepends.Wait()
	if err != nil {
		return err
	}

	ans := qdepends.ProcessState.ExitCode()
	if ans != 0 {
		return errors.New("Error on running rdepends for package " + pkg + ": " + errBuffer.String())
	}

	out := outBuffer.String()
	if len(out) > 0 {
		// Drop prefix
		out = out[6:]

		// Multiple match returns multiple rows. I get the first.
		rows := strings.Split(out, "\n")
		if len(rows) > 1 {
			out = rows[0]
		}

		deps := strings.Split(out, " ")

		for _, dep := range deps {

			originalDep := dep

			// Drop garbage string
			if len(dep) == 0 {
				continue
			}

			dep = strings.Trim(dep, "\n")
			dep = strings.Trim(dep, "\r")

			if strings.Index(dep, ":") > 0 {

				depWithoutSlot := dep[0:strings.Index(dep, ":")]
				slot := dep[strings.Index(dep, ":")+1:]
				// i found slot but i want drop all subslot
				if strings.Index(slot, "/") > 0 {
					slot = slot[0:strings.Index(slot, "/")]
				}
				dep = depWithoutSlot + ":" + slot
			}

			gp, err := gentoo.ParsePackageStr(dep)
			if err != nil {
				return errors.New("On convert dep " + dep + ": " + err.Error())
			}

			fmt.Println(fmt.Sprintf("[%s] Resolving dep '%s' -> %s...",
				solution.Package.GetPackageName(), originalDep,
				gp.GetPackageName()))
			SanitizeSlot(gp)
			if runtime {
				if gp.Condition == gentoo.PkgCondNot {
					solution.RuntimeConflicts = append(solution.RuntimeConflicts, *gp)
				} else {
					solution.RuntimeDeps = append(solution.RuntimeDeps, *gp)
				}
			} else {
				if gp.Condition == gentoo.PkgCondNot {
					solution.BuildConflicts = append(solution.BuildConflicts, *gp)
				} else {
					solution.BuildDeps = append(solution.BuildDeps, *gp)
				}
			}
		}

	} else {
		typeDeps := "build-time"
		if runtime {
			typeDeps = "runtime"
		}
		fmt.Println(fmt.Sprintf("No %s dependencies found for package %s.",
			typeDeps, solution.Package.GetPackageName()))
	}

	return nil
}

func (r *QDependsResolver) Resolve(pkg string, opts *specs.PortageResolverOpts) (*specs.PortageSolution, error) {
	ans := &specs.PortageSolution{
		BuildDeps:        make([]gentoo.GentooPackage, 0),
		RuntimeDeps:      make([]gentoo.GentooPackage, 0),
		BuildConflicts:   make([]gentoo.GentooPackage, 0),
		RuntimeConflicts: make([]gentoo.GentooPackage, 0),
		Labels:           make(map[string]string, 0),
	}

	gp, err := gentoo.ParsePackageStr(pkg)
	if err != nil {
		return nil, err
	}

	ans.Package = *gp

	// Retrive last version
	err = retrieveVersion(ans)
	if err != nil {
		// If with slot trying to use a package without slot
		if strings.Index(pkg, ":") > 0 {
			pkg = pkg[0:strings.Index(pkg, ":")]
			gp, err = gentoo.ParsePackageStr(pkg)
			if err != nil {
				return nil, err
			}

			ans.Package = *gp
			err = retrieveVersion(ans)
			if err != nil {
				return nil, err
			}

		} else {
			return nil, err
		}
	}

	// Retrieve runtime deps
	err = runQdepends(ans, true)
	if err != nil {
		return nil, err
	}

	// Retrieve build-time deps
	err = runQdepends(ans, false)
	if err != nil {
		return nil, err
	}

	// Sanitize slot
	SanitizeSlot(&ans.Package)

	return ans, nil
}
