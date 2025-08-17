/*
Copyright © 2021-2024 Macaroni OS Linux
See AUTHORS and LICENSE for the license details and contributors.
*/
package reposcan

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	helpers "github.com/MottainaiCI/lxd-compose/pkg/helpers"
	. "github.com/geaaru/luet/pkg/logger"
	gentoo "github.com/geaaru/pkgs-checker/pkg/gentoo"

	"golang.org/x/sync/semaphore"
)

var (
	MetadataLines = []string{
		"DEPEND",
		"RDEPEND",
		"SLOT",
		"SRC_URI",
		"RESTRICT",
		"HOMEPAGE",
		"LICENSE",
		"DESCRIPTION",
		"KEYWORDS",
		"INHERITED",
		"IUSE",
		"REQUIRED_USE",
		"PDEPEND",
		"BDEPEND",
		"EAPI",
		"PROPERTIES",
		"DEFINED_PHASES",
		"HDEPEND",
		"PYTHON_COMPAT",
	}
)

// NOTE: RepoScanGenerator is intended to parse
//
//	at max one kit for structure. You nee
type RepoScanGenerator struct {
	PortageBinPath string `json:"portage_bin_path,omitempty" yaml:"portage_bin_path,omitempty"`

	EclassesPaths        []string `json:"eclass_paths,omitempty" yaml:"eclass_paths,omitempty"`
	PythonImplementation []string `json:"python_implementations,omitempty" yaml:"python_implementations,omitempty"`

	Counter int `json:"counter" yaml:"counter"`
	Errors  int `json:"errors" yaml:"errors"`

	Concurrency int `json:"-" yaml:"-"`

	EclassesHashMap map[string]string   `json:"-" yaml:"-"`
	mutex           sync.Mutex          `json:"-" yaml:"-"`
	semaphore       *semaphore.Weighted `json:"-" yaml:"-"`
	waitGroup       *sync.WaitGroup     `json:"-" yaml:"-"`
	ctx             context.Context     `json:"-" yaml:"-"`
}

type ChannelGenerateRes struct {
	Pkgdir string
	Error  error
}

func NewRepoScanGenerator(portage_bpath string) *RepoScanGenerator {
	return &RepoScanGenerator{
		PortageBinPath:  portage_bpath,
		EclassesPaths:   []string{},
		EclassesHashMap: make(map[string]string, 0),
		PythonImplementation: []string{
			"python2_7",
			"python3_7",
			"python3_8",
			"python3_9",
			"python3_10",
			"python3_11",
			"python3_12",
		},
	}
}

func (r *RepoScanGenerator) SetConcurrency(c int) { r.Concurrency = c }
func (r *RepoScanGenerator) GetConcurrency() int  { return r.Concurrency }

func (r *RepoScanGenerator) GetEclassHash(fpath string) string {
	ans := ""

	r.mutex.Lock()
	defer r.mutex.Unlock()

	if hash, present := r.EclassesHashMap[fpath]; present {
		ans = hash
	}

	return ans
}

func (r *RepoScanGenerator) loadEclassesMap(eclassDir string) error {
	eclassDir = filepath.Join(eclassDir, "eclass")
	files, err := os.ReadDir(eclassDir)
	if err != nil {
		return err
	}

	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".eclass") {
			continue
		}

		eclasspath := filepath.Join(eclassDir, file.Name())

		content, err := os.ReadFile(eclasspath)
		if err != nil {
			return err
		}

		hash := fmt.Sprintf("%x", md5.Sum(content))

		r.SetEclassHash(eclasspath, hash)
	}

	return nil
}

func (r *RepoScanGenerator) SetEclassHash(fpath, hash string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.EclassesHashMap[fpath] = hash
}

func (r *RepoScanGenerator) AddEclassesPath(p string) {
	present := false
	for _, s := range r.EclassesPaths {
		if s == p {
			present = true
			break
		}
	}

	if !present {
		if helpers.Exists(p) {
			// Load all eclasses
			err := r.loadEclassesMap(p)
			if err != nil {
				Warning(fmt.Sprintf(
					"Error on load eclasses map for dir %s: %s", p, err.Error()))
			}

			r.EclassesPaths = append(r.EclassesPaths, p)
		}
	}
}

func (r *RepoScanGenerator) getUsedEclass(ebuildPath, eclassName string) (string, error) {
	ans := ""
	eclassFile := eclassName + ".eclass"

	localEclassDir := filepath.Join(filepath.Dir(ebuildPath), "../../eclass")
	localEclass := filepath.Join(localEclassDir, eclassFile)

	if helpers.Exists(localEclass) {
		ans = localEclass
	} else {

		idx := len(r.EclassesPaths) - 1
		for idx >= 0 {

			eclassdir := filepath.Join(r.EclassesPaths[idx], "eclass/")

			if eclassdir == localEclass {
				continue
			}

			eclassPath := filepath.Join(eclassdir, eclassFile)
			if helpers.Exists(eclassPath) {
				ans = eclassPath
				break
			}
			idx--
		}

	}

	return ans, nil
}

func (r *RepoScanGenerator) ProcessKit(kit, branch, path string) (*RepoScanSpec, error) {
	ans := &RepoScanSpec{
		CacheDataVersion: CacheDataVersion,
		Atoms:            make(map[string]RepoScanAtom, 0),
		MetadataErrors:   make(map[string]RepoScanAtom, 0),
		File:             "",
	}

	r.semaphore = semaphore.NewWeighted(int64(r.Concurrency))
	r.waitGroup = &sync.WaitGroup{}
	r.ctx = context.TODO()

	var ch chan ChannelGenerateRes = make(
		chan ChannelGenerateRes,
		r.Concurrency,
	)

	dirEntries, err := os.ReadDir(path)
	if err != nil {
		return ans, fmt.Errorf("Error on read dir %s: %s", path, err.Error())
	}

	eclassesdir := filepath.Join(path, "eclass")
	if helpers.Exists(eclassesdir) {
		// NOTE: The eclass path must without eclass/"
		r.AddEclassesPath(path)
	}

	nPkgs := 0
	// Iterate over every category directory
	for _, cat := range dirEntries {

		if !cat.IsDir() || cat.Name() == "eclass" ||
			cat.Name() == "profiles" || cat.Name() == "licenses" {
			continue
		}

		// Ignoring git directory
		if strings.HasPrefix(cat.Name(), ".") {
			continue
		}

		catDir := filepath.Join(path, cat.Name())

		pkgDirsEntries, err := os.ReadDir(catDir)
		if err != nil {
			return ans, fmt.Errorf("Error on read dir %s: %s", catDir, err.Error())
		}

		for _, pkg := range pkgDirsEntries {
			if !pkg.IsDir() {
				continue
			}

			r.waitGroup.Add(1)
			nPkgs++

			go r.processPackageDir(filepath.Join(path, cat.Name(), pkg.Name()),
				cat.Name(), pkg.Name(), ans, kit, branch, ch)
			if err != nil {
				return ans, fmt.Errorf("Error on process package %s: %s", pkg, err.Error())
			}
		}

	}

	//fmt.Println("Elaborating", nPkgs, "...")

	if nPkgs > 0 {
		for i := 0; i < nPkgs; i++ {
			resp := <-ch
			if resp.Error != nil {
				Error(fmt.Sprintf(
					"On package dir %s: %s", resp.Pkgdir, resp.Error))
			}
		}
		r.waitGroup.Wait()
	}

	return ans, nil
}

func (r *RepoScanGenerator) processPackageDir(path, cat, pkg string,
	spec *RepoScanSpec, kit, branch string, ch chan ChannelGenerateRes) {

	var manifest *ManifestFile
	var err error
	mfile := filepath.Join(path, "Manifest")

	//fmt.Println("Elaborating package dir", path)

	defer r.waitGroup.Done()
	err = r.semaphore.Acquire(r.ctx, 1)
	if err != nil {
		ch <- ChannelGenerateRes{
			Error:  err,
			Pkgdir: path,
		}
		return
	}
	defer r.semaphore.Release(1)

	//fmt.Println("Elaborating package ", pkg)

	// The data available on Manifest file are needed to
	// popolate the RepoScanAtom, so I need to parse it before
	// the other if the file exists.
	if helpers.Exists(mfile) {
		manifest, err = ParseManifest(mfile)
		if err != nil {
			ch <- ChannelGenerateRes{
				Error:  err,
				Pkgdir: path,
			}
			return
		}
	}

	files, err := os.ReadDir(path)
	if err != nil {
		ch <- ChannelGenerateRes{
			Error:  err,
			Pkgdir: path,
		}
		return
	}

	for _, file := range files {
		if file.Name() == "Manifest" {
			// Manifest is already been processed.
			continue
		}

		if filepath.Ext(file.Name()) == ".ebuild" {
			ebuildPath := filepath.Join(path, file.Name())
			atom := strings.ReplaceAll(file.Name(), ".ebuild", "")
			atom = fmt.Sprintf("%s/%s", cat, atom)
			m, err := r.GetEbuildMetadata(ebuildPath, manifest)
			if err != nil {
				r.addAtomError(spec,
					&RepoScanAtom{
						Kit:    kit,
						Branch: branch,
						Status: "ebuild.sh failure",
						Output: err.Error(),
					}, atom)
				continue
			}

			// Retrieve ebuild md5
			content, err := os.ReadFile(ebuildPath)
			if err != nil {
				r.addAtomError(spec, &RepoScanAtom{
					Kit:    kit,
					Branch: branch,
					Status: "read content failure",
					Output: err.Error(),
				}, atom)
				continue
			}

			ra := &RepoScanAtom{
				Kit:      kit,
				Branch:   branch,
				Atom:     atom,
				CatPkg:   fmt.Sprintf("%s/%s", cat, pkg),
				Package:  pkg,
				Category: cat,
				Metadata: m,
				Md5:      fmt.Sprintf("%x", md5.Sum(content)),
			}
			if manifest != nil {
				ra.ManifestMd5 = manifest.Md5
			}

			g, err := gentoo.ParsePackageStr(atom)
			if err != nil {
				r.addAtomError(spec, &RepoScanAtom{
					Kit:    kit,
					Branch: branch,
					Status: "error on parse package string",
					Output: err.Error(),
				}, atom)
				continue
			}

			ra.Relations = []string{}
			ra.RelationsByKind = make(map[string][]string, 0)

			if strings.HasPrefix(g.VersionSuffix, "-r") {
				ra.Revision = g.VersionSuffix[2:]
			} else {
				ra.Revision = "0"
			}

			val, _ := m["INHERITED"]
			if val != "" {
				eclasses := strings.Split(val, " ")
				for _, e := range eclasses {
					eclassPath, err := r.getUsedEclass(ebuildPath, e)
					if err != nil {
						Warning(fmt.Sprintf("[%s] Error on resolve eclass %s: %s",
							atom, e, err.Error()))
						continue
					}

					hash := r.GetEclassHash(eclassPath)
					ra.Eclasses = append(ra.Eclasses,
						[]string{e, hash})
				}
			}

			ra.MetadataOut = m["METADATA_OUT"]
			delete(ra.Metadata, "METADATA_OUT")

			if manifest != nil {
				ra.Files, err = manifest.GetFiles(m["SRC_URI"])
				if err != nil {
					Warning(fmt.Sprintf("[%s] error on package manifest files: %s",
						ra.Atom, err.Error()))
				}
			}

			// Try to populate relations and relations_by_kind
			if depsStr, present := m["BDEPEND"]; present {
				if strings.TrimSpace(depsStr) != "" {
					err := r.retrieveRelations("BDEPEND", depsStr, ra)
					if err != nil {
						Warning(fmt.Sprintf(
							"[%s] error on parse dependendies %s: %s",
							ra.Atom, "BDEPEND", err.Error()))
					}
				}
			}

			if depsStr, present := m["RDEPEND"]; present {
				if strings.TrimSpace(depsStr) != "" {
					err := r.retrieveRelations("RDEPEND", depsStr, ra)
					if err != nil {
						Warning(fmt.Sprintf(
							"[%s] error on parse dependendies %s: %s",
							ra.Atom, "RDEPEND", err.Error()))
					}
				}
			}

			if depsStr, present := m["DEPEND"]; present {
				if strings.TrimSpace(depsStr) != "" {
					err := r.retrieveRelations("DEPEND", depsStr, ra)
					if err != nil {
						Warning(fmt.Sprintf(
							"[%s] error on parse dependendies %s: %s",
							ra.Atom, "DEPEND", err.Error()))
					}
				}
			}

			if depsStr, present := m["PDEPEND"]; present {
				if strings.TrimSpace(depsStr) != "" {
					err := r.retrieveRelations("PDEPEND", depsStr, ra)
					if err != nil {
						Warning(fmt.Sprintf(
							"[%s] error on parse dependendies %s: %s",
							ra.Atom, "PDEPEND", err.Error()))
					}
				}
			}

			r.addAtom(spec, ra)
		}
	}

	ch <- ChannelGenerateRes{
		Error:  nil,
		Pkgdir: path,
	}
	return
}

func (r *RepoScanGenerator) retrieveRelations(rtype string, depsStr string, atom *RepoScanAtom) error {

	deps, err := ParseDependencies(depsStr)
	if err != nil {
		return err
	}

	for _, du := range deps.Dependencies {

		if du.Dep != nil {
			if du.Dep.Condition == gentoo.PkgCondNot {
				continue
			}

			atom.AddRelations(du.Dep.GetPackageName())
			atom.AddRelationsByKind(rtype, du.Dep.GetPackageName())
		}

		depsList := du.GetDepsList()
		for _, d := range depsList {
			if d.Dep.Condition == gentoo.PkgCondNot {
				continue
			}

			atom.AddRelations(d.Dep.GetPackageName())
			atom.AddRelationsByKind(rtype, d.Dep.GetPackageName())
		}
	}

	return nil
}

func (r *RepoScanGenerator) addAtom(spec *RepoScanSpec, atom *RepoScanAtom) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	spec.Atoms[atom.Atom] = *atom
	r.Counter++
}

func (r *RepoScanGenerator) addAtomError(spec *RepoScanSpec,
	atom *RepoScanAtom, astr string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	spec.MetadataErrors[astr] = *atom
	r.Errors++
}

func (r *RepoScanGenerator) GetEbuildMetadata(
	file string, manifest *ManifestFile) (map[string]string, error) {
	ans := make(map[string]string, 0)
	var outBuffer, errBuffer bytes.Buffer

	ebuildsh := filepath.Join(r.PortageBinPath, "ebuild.sh")

	// Prepare envs for ebuild.sh
	envs := []string{
		"PORTAGE_BIN_PATH=" + r.PortageBinPath,
		"EBUILD_PHASE=depend",
		"ECLASS_DEBUG_OUTPUT=on",
		"PORTAGE_GID=250",
		"EAPI=012345678",
		"LC_COLLATE=POSIX",
		"PATH=/bin:/usr/bin",
		"PORTAGE_PIPE_FD=1",
		"EBUILD=" + file,
	}

	// Set PORTAGE_ECLASS_LOCATIONS.
	// NOTE: The paths must be without eclass/ subdir.
	envs = append(envs,
		"PORTAGE_ECLASS_LOCATIONS="+strings.Join(r.EclassesPaths, " "))

	// Prepare gentoo package to retrieve ebuild envs
	atoms := strings.Split(file, "/")
	pkgstr := atoms[len(atoms)-3] + "/" +
		strings.ReplaceAll(atoms[len(atoms)-1], ".ebuild", "")
	g, err := gentoo.ParsePackageStr(pkgstr)
	if err != nil {
		return ans, err
	}
	envs = append(envs, []string{
		"PV=" + g.GetPV(),
		"PN=" + g.GetPN(),
		"P=" + g.GetP(),
		"PVR=" + g.GetPVR(),
		"PF=" + g.GetPF(),
		"CATEGORY=" + atoms[len(atoms)-3],
	}...)

	entrypoint := []string{"/bin/bash", "-c"}
	cmds := append(entrypoint, fmt.Sprintf("source %s", ebuildsh))

	cmd := exec.Command(cmds[0], cmds[1:]...)

	cmd.Stdout = helpers.NewNopCloseWriter(&outBuffer)
	cmd.Stderr = helpers.NewNopCloseWriter(&errBuffer)
	cmd.Env = envs

	err = cmd.Start()
	if err != nil {
		return ans, fmt.Errorf("error on start command: %s", err.Error())
	}

	err = cmd.Wait()
	if err != nil {
		return ans, fmt.Errorf("%s: %s", err.Error(), errBuffer.String())
	}

	res := cmd.ProcessState.ExitCode()
	if res != 0 {
		return ans, fmt.Errorf("[%s] Error on retrieve metadata: %s",
			pkgstr, errBuffer.String())
	}

	lines := strings.Split(outBuffer.String(), "\n")
	if len(lines) < len(MetadataLines) {
		return ans, fmt.Errorf(
			"[%s] Unexpected number of lines from ebuild.sh (%d expected %d)",
			pkgstr, len(lines), len(MetadataLines))
	}

	for idx, k := range MetadataLines {
		ans[k] = lines[idx]
	}
	ans["METADATA_OUT"] = outBuffer.String()

	return ans, nil
}

func (r *RepoScanGenerator) ParseAtom(ebuildFile, cat, pkg, kit, branch string) (*RepoScanAtom, error) {
	ans := &RepoScanAtom{}

	var manifest *ManifestFile
	var err error
	mfile := filepath.Join(filepath.Dir(ebuildFile), "Manifest")

	// The data available on Manifest file are needed to
	// popolate the RepoScanAtom, so I need to parse it before
	// the other if the file exists.
	if helpers.Exists(mfile) {
		manifest, err = ParseManifest(mfile)
		if err != nil {
			return ans, fmt.Errorf("error on parse manifest file %s: %s",
				mfile, err.Error())
		}
	}

	fileName := filepath.Base(ebuildFile)
	pnpv := strings.ReplaceAll(fileName, ".ebuild", "")
	m, err := r.GetEbuildMetadata(ebuildFile, manifest)
	if err != nil {
		return ans, fmt.Errorf("error on parse ebuild file %s: %s",
			ebuildFile, err.Error())
	}

	// Retrieve ebuild md5
	content, err := os.ReadFile(ebuildFile)
	if err != nil {
		return ans, fmt.Errorf("error on parse file %s: %s",
			ebuildFile, err.Error())
	}

	ans.Kit = kit
	ans.Branch = branch
	ans.Atom = fmt.Sprintf("%s/%s", cat, pnpv)
	ans.CatPkg = fmt.Sprintf("%s/%s", cat, pkg)
	ans.Package = pkg
	ans.Category = cat
	ans.Metadata = m
	if manifest != nil {
		ans.ManifestMd5 = manifest.Md5
	}
	ans.Md5 = fmt.Sprintf("%x", md5.Sum(content))

	g, err := gentoo.ParsePackageStr(ans.Atom)
	if err != nil {
		return ans, fmt.Errorf("error on parse package string %s: %s",
			ans.Atom, err.Error())
	}
	if strings.HasPrefix(g.VersionSuffix, "-r") {
		ans.Revision = g.VersionSuffix[2:]
	} else {
		ans.Revision = "0"
	}

	val, _ := m["INHERITED"]
	if val != "" {
		eclasses := strings.Split(val, " ")
		for _, e := range eclasses {

			eclassPath, err := r.getUsedEclass(ebuildFile, e)
			if err != nil {
				Warning(fmt.Sprintf("[%s] Error on resolve eclass %s: %s",
					ans.Atom, e, err.Error()))
				continue
			}

			hash := r.GetEclassHash(eclassPath)

			// For now I ignoring eclass hashes
			ans.Eclasses = append(ans.Eclasses,
				[]string{e, hash})

		}
	}

	ans.MetadataOut = m["METADATA_OUT"]
	delete(ans.Metadata, "METADATA_OUT")

	ans.Files, err = manifest.GetFiles(m["SRC_URI"])
	if err != nil {
		Warning(fmt.Sprintf("[%s] error on package manifest files: %s",
			ans.Atom, err.Error()))
	}

	ans.Relations = []string{}
	ans.RelationsByKind = make(map[string][]string, 0)

	// Try to populate relations and relations_by_kind
	if depsStr, present := m["BDEPEND"]; present {
		if strings.TrimSpace(depsStr) != "" {
			err := r.retrieveRelations("BDEPEND", depsStr, ans)
			if err != nil {
				Warning(fmt.Sprintf("[%s] error on parse dependendies %s: %s",
					ans.Atom, "BDEPEND", err.Error()))
			}
		}
	}

	if depsStr, present := m["RDEPEND"]; present {
		if strings.TrimSpace(depsStr) != "" {
			err := r.retrieveRelations("RDEPEND", depsStr, ans)
			if err != nil {
				Warning(fmt.Sprintf("[%s] error on parse dependendies %s: %s",
					ans.Atom, "RDEPEND", err.Error()))
			}
		}
	}

	if depsStr, present := m["DEPEND"]; present {
		if strings.TrimSpace(depsStr) != "" {
			err := r.retrieveRelations("DEPEND", depsStr, ans)
			if err != nil {
				Warning(fmt.Sprintf("[%s] error on parse dependendies %s: %s",
					ans.Atom, "DEPEND", err.Error()))
			}
		}
	}

	if depsStr, present := m["PDEPEND"]; present {
		if strings.TrimSpace(depsStr) != "" {
			err := r.retrieveRelations("PDEPEND", depsStr, ans)
			if err != nil {
				Warning(fmt.Sprintf("[%s] error on parse dependendies %s: %s",
					ans.Atom, "PDEPEND", err.Error()))
			}
		}
	}

	return ans, nil
}
