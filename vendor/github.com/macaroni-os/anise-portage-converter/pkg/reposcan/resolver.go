/*
Copyright © 2021-2023 Macaroni OS Linux
See AUTHORS and LICENSE for the license details and contributors.
*/
package reposcan

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/macaroni-os/anise-portage-converter/pkg/specs"

	. "github.com/geaaru/luet/pkg/logger"
	gentoo "github.com/geaaru/pkgs-checker/pkg/gentoo"
)

type RepoScanResolver struct {
	JsonSources        []string
	Sources            []RepoScanSpec
	Constraints        []string
	MapConstraints     map[string]([]gentoo.GentooPackage)
	Map                map[string]([]RepoScanAtom)
	IgnoreMissingDeps  bool
	ContinueWithError  bool
	DepsWithSlot       bool
	AllowEmptyKeywords bool
	DisabledUseFlags   []string
	DisabledKeywords   []string
}

func NewRepoScanResolver() *RepoScanResolver {
	return &RepoScanResolver{
		JsonSources:        make([]string, 0),
		Sources:            make([]RepoScanSpec, 0),
		Constraints:        make([]string, 0),
		MapConstraints:     make(map[string][]gentoo.GentooPackage, 0),
		Map:                make(map[string][]RepoScanAtom, 0),
		IgnoreMissingDeps:  false,
		DepsWithSlot:       true,
		ContinueWithError:  true,
		AllowEmptyKeywords: false,
	}
}

func (r *RepoScanResolver) SetContinueWithError(v bool) { r.ContinueWithError = v }
func (r *RepoScanResolver) GetContinueWithError() bool  { return r.ContinueWithError }

func (r *RepoScanResolver) SetIgnoreMissingDeps(v bool)    { r.IgnoreMissingDeps = v }
func (r *RepoScanResolver) IsIgnoreMissingDeps() bool      { return r.IgnoreMissingDeps }
func (r *RepoScanResolver) SetDepsWithSlot(v bool)         { r.DepsWithSlot = v }
func (r *RepoScanResolver) GetDepsWithSlot() bool          { return r.DepsWithSlot }
func (r *RepoScanResolver) SetDisabledUseFlags(u []string) { r.DisabledUseFlags = u }
func (r *RepoScanResolver) GetDisabledUseFlags() []string  { return r.DisabledUseFlags }
func (r *RepoScanResolver) SetDisabledKeywords(k []string) { r.DisabledKeywords = k }
func (r *RepoScanResolver) GetDisabledKeywords() []string  { return r.DisabledKeywords }
func (r *RepoScanResolver) SetAllowEmptyKeywords(v bool)   { r.AllowEmptyKeywords = v }
func (r *RepoScanResolver) GetAllowEmptyKeywords() bool    { return r.AllowEmptyKeywords }
func (r *RepoScanResolver) IsDisableUseFlag(u string) bool {
	ans := false

	if len(r.DisabledUseFlags) > 0 {
		for _, useFlag := range r.DisabledUseFlags {
			if useFlag == u {
				ans = true
				break
			}
		}
	}

	return ans
}

func (r *RepoScanResolver) LoadJson(path string) error {
	fd, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fd.Close()

	decoder := json.NewDecoder(fd)

	for {
		var spec RepoScanSpec
		if err := decoder.Decode(&spec); err == io.EOF {
			break
		} else if err != nil {
			return err
		}
		spec.File = path
		r.Sources = append(r.Sources, spec)
	}

	return nil
}

func (r *RepoScanResolver) LoadRawJson(raw, file string) error {
	var spec RepoScanSpec
	err := json.Unmarshal([]byte(raw), &spec)
	if err != nil {
		return err
	}
	spec.File = file

	r.Sources = append(r.Sources, spec)

	return nil
}

func (r *RepoScanResolver) LoadJsonFiles(verbose bool) error {
	for _, file := range r.JsonSources {
		if verbose {
			InfoC(fmt.Sprintf(":brain: Loading reposcan file %s...", file))
		}
		err := r.LoadJson(file)
		if err != nil {
			return err
		}
	}

	// Create packages map
	return nil
}

func (r *RepoScanResolver) BuildMap() error {
	//fmt.Println("Build MAP ")
	for idx, _ := range r.Sources {

		for pkg, atom := range r.Sources[idx].Atoms {

			p := atom.CatPkg

			if atom.Status != "" {
				Warning(fmt.Sprintf(":warn Skipping pkg %s with wrong status.", pkg))
				// Skip broken atoms
				continue
			}

			if val, ok := r.Map[p]; ok {

				atomref := r.Sources[idx].Atoms[pkg]
				// POST: entry found
				r.Map[p] = append(val, atomref)

			} else {
				atomref := r.Sources[idx].Atoms[pkg]
				// POST: no entry available.
				r.Map[p] = []RepoScanAtom{atomref}
			}
		}
	}

	// Build contraints Map
	if len(r.Constraints) > 0 {
		for _, c := range r.Constraints {
			gp, err := gentoo.ParsePackageStr(c)
			if err != nil {
				return err
			}

			if val, ok := r.MapConstraints[gp.GetPackageName()]; ok {
				r.MapConstraints[gp.GetPackageName()] = append(val, *gp)
			} else {
				r.MapConstraints[gp.GetPackageName()] = []gentoo.GentooPackage{*gp}
			}

		}
	}

	return nil
}

func (r *RepoScanResolver) GetMap() map[string]([]RepoScanAtom) {
	return r.Map
}

func (r *RepoScanResolver) IsPresentPackage(pkg string) bool {
	_, ok := r.Map[pkg]
	return ok
}

func (r *RepoScanResolver) Resolve(pkg string, opts *specs.PortageResolverOpts) (*specs.PortageSolution, error) {

	if pkg == "" {
		return nil, errors.New("Invalid pkg to resolve")
	}

	ans := &specs.PortageSolution{
		BuildDeps:        make([]gentoo.GentooPackage, 0),
		RuntimeDeps:      make([]gentoo.GentooPackage, 0),
		BuildConflicts:   make([]gentoo.GentooPackage, 0),
		RuntimeConflicts: make([]gentoo.GentooPackage, 0),
		Labels:           make(map[string]string, 0),
		Uri:              []string{},
		Description:      "",
	}

	// Retrive last version
	atom, err := r.GetLastPackage(pkg, opts)
	if err != nil {
		return nil, err
	}

	last, err := atom.ToGentooPackage()
	if err != nil {
		return nil, err
	}

	ans.Package = *last

	ans.Description = atom.GetMetadataValue("DESCRIPTION")
	if atom.HasMetadataKey("HOMEPAGE") {
		// Temporary workaround of reposcan generation
		uris := strings.Split(atom.GetMetadataValue("HOMEPAGE"), " ")
		for _, u := range uris {
			if u != "" && u != "None" {
				ans.Uri = append(ans.Uri, u)
			}
		}
	}
	if atom.HasMetadataKey("IUSE") {
		ans.SetLabel("IUSE", atom.GetMetadataValue("IUSE"))
	}

	DebugC(fmt.Sprintf("[%s] Retrieve Runtime Dependencies...", atom.Atom))
	err = r.retrieveRuntimeDeps(atom, last, ans, opts)
	if err != nil {
		return nil, err
	}

	DebugC(fmt.Sprintf("[%s] Retrieve Buildtime Dependencies...", atom.Atom))
	err = r.retrieveBuildtimeDeps(atom, last, ans, opts)
	if err != nil {
		return nil, err
	}

	return ans, nil
}

func (r *RepoScanResolver) retrieveRuntimeDeps(atom *RepoScanAtom,
	last *gentoo.GentooPackage, solution *specs.PortageSolution,
	opts *specs.PortageResolverOpts) error {

	var err error
	var rdeps []gentoo.GentooPackage
	var conflicts []gentoo.GentooPackage

	// Trying to elaborate use flags and use old way as failover
	if atom.HasMetadataKey("RDEPEND") {
		rdepend := atom.GetMetadataValue("RDEPEND")
		solution.SetLabel("RDEPEND", rdepend)

		DebugC(fmt.Sprintf("[%s] RDEPEND %s", atom.Atom, rdepend))

		deps, err := ParseDependencies(rdepend)
		if err != nil {
			Warning(fmt.Sprintf("[%s] RDEPEND Error on parsing '%s': %s", atom.Atom, rdepend, err.Error()))
			Warning("Using relations directly.")

			rdeps, err = atom.GetRuntimeDeps()
			if err != nil {
				return err
			}
		} else {

			rdeps, conflicts, err = r.elaborateDepsAndUseFlags(deps, opts)
			if err != nil {
				return err
			}

			// Retrieve the use flags
			useFlags := deps.GetUseFlags()
			r.assignUseFlags(solution, useFlags, opts)
		}

	}

	if len(conflicts) > 0 {
		conflicts, err = r.elaborateConflicts(last, conflicts, opts)
		if err != nil {
			return err
		}
		solution.RuntimeConflicts = append(solution.RuntimeConflicts, conflicts...)
	}

	if len(rdeps) > 0 {

		rdeps, err = r.elaborateDeps(last, rdeps, opts)
		if err != nil {
			return err
		}

		solution.RuntimeDeps = append(solution.RuntimeDeps, rdeps...)
	}

	return nil
}

func (r *RepoScanResolver) retrieveBuildtimeDeps(atom *RepoScanAtom, last *gentoo.GentooPackage, solution *specs.PortageSolution, opts *specs.PortageResolverOpts) error {
	var err error
	bdeps := []gentoo.GentooPackage{}
	conflicts := []gentoo.GentooPackage{}

	// Trying to elaborate use flags and use old way as failover
	if atom.HasMetadataKey("DEPEND") {
		depend := atom.GetMetadataValue("DEPEND")
		solution.SetLabel("DEPEND", depend)

		DebugC(fmt.Sprintf("[%s] DEPEND %s", atom.Atom, depend))

		deps, err := ParseDependencies(depend)
		if err != nil {
			Warning(fmt.Sprintf("[%s] DEPEND Error on parsing '%s': %s", atom.Atom, depend, err.Error()))
			Warning("Using relations directly.")

			bdeps, err = atom.GetBuildtimeDeps()
			if err != nil {
				return err
			}
		} else {
			// Retrieve the use flags
			useFlags := deps.GetUseFlags()
			r.assignUseFlags(solution, useFlags, opts)

			bdeps, conflicts, err = r.elaborateDepsAndUseFlags(deps, opts)
			if err != nil {
				return err
			}

		}

	}

	if atom.HasMetadataKey("BDEPEND") {
		bdepends := atom.GetMetadataValue("BDEPEND")
		solution.SetLabel("BDEPEND", bdepends)

		DebugC(fmt.Sprintf("[%s] BDEPEND %s", atom.Atom, bdepends))

		deps, err := ParseDependencies(bdepends)
		if err != nil {
			Warning(fmt.Sprintf("[%s] BDEPEND: Error on parsing '%s': %s", atom.Atom, bdepends, err.Error()))
			Warning("Using relations directly.")

			bdeps, err = atom.GetBuildtimeDeps()
			if err != nil {
				return err
			}
		} else {

			// Retrieve the use flags
			useFlags := deps.GetUseFlags()
			r.assignUseFlags(solution, useFlags, opts)

			d, c, err := r.elaborateDepsAndUseFlags(deps, opts)
			if err != nil {
				return err
			}

			if len(d) > 0 {
				bdeps = append(bdeps, d...)
			}

			if len(c) > 0 {
				conflicts = append(conflicts, c...)
			}
		}

	}

	if len(conflicts) > 0 {
		conflicts, err = r.elaborateConflicts(last, conflicts, opts)
		if err != nil {
			return err
		}
		solution.BuildConflicts = append(solution.BuildConflicts, conflicts...)
	}

	if len(bdeps) > 0 {

		bdeps, err = r.elaborateDeps(last, bdeps, opts)
		if err != nil {
			return err
		}

		solution.BuildDeps = append(solution.BuildDeps, bdeps...)
	}

	return nil
}

func (r *RepoScanResolver) elaborateDepsAndUseFlags(s *EbuildDependencies, opts *specs.PortageResolverOpts) ([]gentoo.GentooPackage, []gentoo.GentooPackage, error) {
	deps := []gentoo.GentooPackage{}
	conflicts := []gentoo.GentooPackage{}

	if len(s.Dependencies) == 0 {
		return deps, conflicts, nil
	}

	for _, gdep := range s.Dependencies {

		// TODO: do this recursive
		d, c, err := r.elaborateGentooDependency(gdep, opts)
		if err != nil {
			return deps, conflicts, err
		}

		if len(d) > 0 {
			deps = append(deps, d...)
		}

		if len(c) > 0 {
			conflicts = append(conflicts, c...)
		}

	}

	return deps, conflicts, nil
}

func (r *RepoScanResolver) elaborateGentooDependency(gdep *GentooDependency, opts *specs.PortageResolverOpts) ([]gentoo.GentooPackage, []gentoo.GentooPackage, error) {
	deps := []gentoo.GentooPackage{}
	conflicts := []gentoo.GentooPackage{}

	if gdep.Use != "" {
		not := gdep.UseCondition == gentoo.PkgCondNot

		toIgnore := r.IsDisableUseFlag(gdep.Use) || !opts.IsAdmitUseFlag(gdep.Use)
		if not {
			toIgnore = opts.IsAdmitUseFlag(gdep.Use)
		}
		// POST: is a use flag GentooDependency
		if toIgnore {
			DebugC(
				GetAurora().Bold(
					fmt.Sprintf("Found dep with use flag %s. Ignoring it.",
						gdep.Use)))
			// Ignore deps
			return deps, conflicts, nil
		}

		if len(gdep.SubDeps) > 0 {
			for _, sdep := range gdep.SubDeps {
				d, c, err := r.elaborateGentooDependency(sdep, opts)
				if err != nil {
					return deps, conflicts, err
				}

				if len(d) > 0 {
					deps = append(deps, d...)
				}

				if len(c) > 0 {
					conflicts = append(conflicts, c...)
				}

			}
		}

	} else if gdep.DepInOr {
		// NOTE: Ignore OR with sub dependency with use flag.

		for _, sdep := range gdep.SubDeps {
			if sdep.Dep == nil {
				// Ignore dep
				continue
			}

			not := sdep.UseCondition == gentoo.PkgCondNot

			toIgnore := r.IsDisableUseFlag(sdep.Use) || !opts.IsAdmitUseFlag(sdep.Use)
			if not {
				toIgnore = opts.IsAdmitUseFlag(sdep.Use)
			}
			// POST: is a use flag GentooDependency
			if toIgnore {
				DebugC(
					GetAurora().Bold(
						fmt.Sprintf("Found sub dep with use flag %s. Ignoring it.",
							sdep.Use)))
				// Ignore deps
				continue
			}

			atom, err := r.GetLastPackage(sdep.Dep.GetPackageNameWithSlot(), opts)
			if err == nil {

				gp, err := atom.ToGentooPackage()
				if err != nil {
					return nil, nil, err
				}

				if !r.DepsWithSlot {
					gp.Slot = ""
				}

				if sdep.UseCondition == gentoo.PkgCondNot {
					conflicts = append(conflicts, *sdep.Dep)
				} else {
					deps = append(deps, *gp)
				}

				break
			}

		}

	}

	if gdep.Dep != nil {

		// Check if current deps is available.
		atom, err := r.GetLastPackage(gdep.Dep.GetPackageNameWithSlot(), opts)
		if err == nil {

			gp, err := atom.ToGentooPackage()
			if err != nil {
				return nil, nil, err
			}

			if !r.DepsWithSlot {
				gp.Slot = ""
			}

			if gdep.Dep.Condition == gentoo.PkgCondNot ||
				gdep.Dep.Condition == gentoo.PkgCondNotLess ||
				gdep.Dep.Condition == gentoo.PkgCondNotGreater {
				conflicts = append(conflicts, *gdep.Dep)
			} else {
				deps = append(deps, *gp)
			}

		}

	}

	return deps, conflicts, nil
}

func (r *RepoScanResolver) elaborateDeps(pkg *gentoo.GentooPackage,
	deps []gentoo.GentooPackage, opts *specs.PortageResolverOpts) ([]gentoo.GentooPackage, error) {
	ans := []gentoo.GentooPackage{}

	for idx, _ := range deps {

		p := deps[idx].GetPackageName()
		if deps[idx].Slot != "" {
			p += ":" + deps[idx].Slot
		}

		atom, err := r.GetLastPackage(p, opts)
		if err != nil {
			if r.IsIgnoreMissingDeps() {
				Warning(
					fmt.Sprintf("[%s] Dependency (%s) %s not found in map. Ignoring it.",
						pkg.GetPackageName(), deps[idx].Condition.String(), deps[idx].GetPackageName()))
				continue
			}

			return nil, err
		}
		gp, err := atom.ToGentooPackage()
		if err != nil {
			return nil, err
		}

		if !r.DepsWithSlot {
			gp.Slot = ""
		}

		ans = append(ans, *gp)
	}

	sort.Sort(gentoo.GentooPackageSorter(ans))

	return ans, nil
}

func (r *RepoScanResolver) elaborateConflicts(pkg *gentoo.GentooPackage,
	deps []gentoo.GentooPackage, opts *specs.PortageResolverOpts) ([]gentoo.GentooPackage, error) {
	ans := []gentoo.GentooPackage{}

	for idx, d := range deps {

		p := deps[idx].GetPackageName()
		if deps[idx].Slot != "" {
			p += ":" + deps[idx].Slot
		}

		_, err := r.GetLastPackage(p, opts)
		if err != nil {
			if r.IsIgnoreMissingDeps() {
				Warning(
					fmt.Sprintf("[%s] Conflict (%s) %s not found in map. Ignoring it.",
						pkg.GetPackageName(), deps[idx].Condition.String(), deps[idx].GetPackageName()))
				continue
			}

			return nil, err
		}

		if !r.DepsWithSlot {
			d.Slot = ""
		}

		ans = append(ans, d)
	}

	sort.Sort(gentoo.GentooPackageSorter(ans))

	return ans, nil
}

func (r *RepoScanResolver) GetLastPackage(pkg string, opts *specs.PortageResolverOpts) (*RepoScanAtom, error) {
	var last *gentoo.GentooPackage
	var ans *RepoScanAtom
	mAtoms := make(map[string]*RepoScanAtom, 0)

	gp, err := gentoo.ParsePackageStr(pkg)
	if err != nil {
		return nil, err
	}
	// Reset slot if not in input
	if strings.Index(pkg, ":") < 0 {
		gp.Slot = ""
	}
	// Ignore sub slot
	if strings.Contains(gp.Slot, "/") {
		gp.Slot = gp.Slot[0:strings.Index(gp.Slot, "/")]
	}

	atoms, ok := r.Map[gp.GetPackageName()]
	if !ok {
		return nil, errors.New(fmt.Sprintf("Package (%s) %s not found in map.",
			gp.Condition.String(), gp.GetPackageName()))
	}

	if len(atoms) > 1 {
		pkgs := []gentoo.GentooPackage{}
		for idx, atom := range atoms {
			p, err := atom.ToGentooPackage()
			if err != nil {
				// If the version is not supported, skip the version
				Warning(fmt.Sprintf(
					"[%s/%s-%s] Error on generate Gentoo package: %s. Package skipped.",
					atom.Category, atom.Package, atom.Revision,
					err.Error()))
				continue
			}

			// TODO: check of handle this in a better way
			valid, err := r.KeywordsIsAdmit(&atom, p)
			if err != nil {
				DebugC(fmt.Sprintf(
					"[%s/%s-%s] Check %s/%s:%s@%s: Invalid keyword.",
					atom.Category, atom.Package, atom.Revision,
					p.Category, p.GetPF(), p.Slot, p.Repository))
			}

			if valid {
				valid, err = r.PackageIsAdmit(gp, p, opts)
				if err != nil {
					Warning(fmt.Sprintf(
						"[%s/%s-%s] %s/%s:%s@%s: Package invalid: %s.",
						atom.Category, atom.Package, atom.Revision,
						p.Category, p.GetPF(), p.Slot, p.Repository, err.Error()))
					continue
				}
			} else {
				DebugC(fmt.Sprintf(
					"[%s/%s-%s] %s/%s:%s@%s: Not valid.",
					atom.Category, atom.Package, atom.Revision,
					p.Category, p.GetPF(), p.Slot, p.Repository))
			}

			DebugC(fmt.Sprintf(
				"[%s/%s:%s] Check (%s) %s/%s:%s@%s: admitted - %v",
				gp.Category, gp.GetPF(), gp.Slot, pkg,
				p.Category, p.GetPF(), p.Slot, p.Repository, valid))

			if valid {
				mAtoms[p.GetPVR()] = &atoms[idx]
				pkgs = append(pkgs, *p)
			}
		}

		if len(pkgs) == 0 {
			return nil, errors.New(fmt.Sprintf("No packages found matching %s", pkg))
		}

		sort.Sort(gentoo.GentooPackageSorter(pkgs))
		last = &pkgs[len(pkgs)-1]
		ans = mAtoms[last.GetPVR()]

	} else {
		availableGp, err := atoms[0].ToGentooPackage()
		if err != nil {
			return nil, err
		}

		// TODO: check of handle this in a better way
		valid, err := r.KeywordsIsAdmit(&atoms[0], availableGp)
		if err != nil {
			DebugC(fmt.Sprintf(
				"[%s/%s-%s] Check %s/%s:%s@%s: Invalid keyword.",
				atoms[0].Category, atoms[0].Package, atoms[0].Revision,
				availableGp.Category, availableGp.GetPF(), availableGp.Slot,
				availableGp.Repository))
		}

		if valid {
			valid, err = r.PackageIsAdmit(gp, availableGp, opts)
			if err != nil {
				return nil, err
			}
		}

		if !valid {
			return nil, errors.New(fmt.Sprintf("No package found matching %s", pkg))
		}
		ans = &atoms[0]
	}

	DebugC(
		fmt.Sprintf("[%s] Using package %s:%s",
			pkg, ans.Atom, ans.GetMetadataValue("SLOT")))

	return ans, nil
}

func (r *RepoScanResolver) PackageIsAdmit(target, atom *gentoo.GentooPackage,
	opts *specs.PortageResolverOpts) (bool, error) {
	valid, err := target.Admit(atom)
	if err != nil {
		return false, err
	}

	if !valid {
		return false, nil
	}

	// Check if atom is admitted by constraints
	if len(r.Constraints) > 0 {

		constraints, ok := r.MapConstraints[target.GetPackageName()]
		if ok {
			admitted := false

			for _, c := range constraints {
				admitted, err = c.Admit(atom)
				if err != nil {
					return false, err
				}
				if admitted {
					break
				}
			}

			if !admitted {
				DebugC(fmt.Sprintf("[%s] Package not admitted by constraints",
					atom.GetPF()))
			}
			valid = admitted
		} else {
			DebugC(fmt.Sprintf("[%s] No constraints found.",
				atom.GetPF()))
		}

	}

	if len(opts.Conditions) > 0 && valid {
		for _, cond := range opts.Conditions {
			p, err := gentoo.ParsePackageStr(cond)
			if err != nil {
				return valid, fmt.Errorf("Package %s has invalid condition %s: %s",
					atom.GetPackageName(), cond, err.Error())
			}

			ok, err := p.Admit(atom)
			if err != nil {
				return valid, fmt.Errorf("Package %s fail on check condition %s: %s",
					atom.GetPackageName(), cond, err.Error())
			}
			if !ok {
				valid = false
				DebugC(fmt.Sprintf("[%s] Package not admitted by condition %s",
					atom.GetPF(), cond))
				break
			}
		}

	}

	return valid, nil
}

func (r *RepoScanResolver) KeywordsIsAdmit(atom *RepoScanAtom, p *gentoo.GentooPackage) (bool, error) {
	ans := true

	keywords := atom.GetMetadataValue("KEYWORDS")
	if keywords == "" && !r.AllowEmptyKeywords {
		if r.ContinueWithError {
			Warning(fmt.Sprintf(
				"[%s] Continue also if KEYWORDS is empty.", atom.Atom))
			return true, nil
		}
		DebugC(fmt.Sprintf(
			"[%s] Skip version without keywords %s or disabled.", atom.Atom, p.GetPF()))
		return false, nil
	}

	DebugC(fmt.Sprintf("[%s] Found KEYWORDS %s", atom.Atom, keywords))

	// On Funtoo it's possible a condition like this:
	// KEYWORDS="-* ~amd64"
	// This means that all keywords are disabled excluded ~amd64.
	// So, if i disabled -* i can accept the keywords with ~amd64.

	if len(r.DisabledKeywords) > 0 {
		ak := strings.Split(keywords, " ")
		for _, k := range ak {
			// We need to check all keywords every time
			for _, d := range r.DisabledKeywords {
				if d == k {
					ans = false
					break
				} else if !ans {
					ans = true
				}
			}
		}

		if !ans {
			DebugC(fmt.Sprintf(
				"[%s] Version %s disabled for keywords %s", atom.Atom, p.GetPF(), keywords))
		}
	}

	DebugC(fmt.Sprintf("[%s] KEYWORDS admit %v", atom.Atom, ans))

	return ans, nil
}

func (r *RepoScanResolver) assignUseFlags(solution *specs.PortageSolution, uFlags []string, opts *specs.PortageResolverOpts) {

	for _, u := range uFlags {
		if opts.IsAdmitUseFlag(u) && !r.IsDisableUseFlag(u) {
			solution.Package.UseFlags = append(solution.Package.UseFlags, u)
		} else {
			solution.Package.UseFlags = append(solution.Package.UseFlags, "-"+u)
		}
	}
}
