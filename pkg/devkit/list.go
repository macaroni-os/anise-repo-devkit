/*
Copyright © 2020-2025 Macaroni OS Linux
See AUTHORS and LICENSE for the license details and contributors.
*/
package devkit

import (
	"fmt"
	"time"

	"github.com/geaaru/pkgs-checker/pkg/gentoo"
	specs "github.com/macaroni-os/anise-repo-devkit/pkg/specs"

	"github.com/macaroni-os/anise/anise-build/pkg/v2/solver"
	anise_config "github.com/macaroni-os/anise/pkg/config"
	. "github.com/macaroni-os/anise/pkg/logger"
	anise_pkg "github.com/macaroni-os/anise/pkg/package"
	pkg "github.com/macaroni-os/anise/pkg/package"
)

type RepoList struct {
	*RepoKnife
}

func NewRepoList(s *specs.AniseRDConfig,
	backend, path string, opts map[string]string) (*RepoList, error) {

	knife, err := NewRepoKnife(s, backend, path, opts)
	if err != nil {
		return nil, err
	}

	ans := &RepoList{
		RepoKnife: knife,
	}

	return ans, nil
}

func (c *RepoList) ListPkgsAvailable() ([]*anise_pkg.DefaultPackage, error) {
	ans := []*anise_pkg.DefaultPackage{}

	err := c.RepoKnife.Analyze()
	if err != nil {
		return nil, err
	}

	for _, art := range c.MetaMap {
		ans = append(ans, art.CompileSpec.Package)
	}

	return ans, nil
}

func (c *RepoList) ListPkgsMissing() ([]*anise_pkg.DefaultPackage, error) {
	ans := []*anise_pkg.DefaultPackage{}
	mPkgs := make(map[string]bool, 0)

	err := c.RepoKnife.Analyze()
	if err != nil {
		return nil, err
	}

	// Creating map with available packages
	for _, art := range c.MetaMap {
		mPkgs[art.CompileSpec.Package.HumanReadableString()] = true
		DebugC(fmt.Sprintf("Find %s",
			art.CompileSpec.Package.HumanReadableString()))
	}

	for _, tree := range c.ForestGuard.GetTrees() {

		for pkg, versions := range tree.Map {

			for idx := range versions {
				p := fmt.Sprintf("%s-%s", pkg, versions[idx])
				DebugC(fmt.Sprintf("Checking %s...", p))

				if _, ok := mPkgs[p]; !ok {

					gpkg, _ := gentoo.ParsePackageStr(p)

					runtimeOpts := solver.NewRuntimeSolverOpts()
					runtimeOpts.NoDeps = true
					s := solver.NewRuntimeSolver(anise_config.AniseCfg, runtimeOpts)
					// Setup the forest guard for the solver
					s.SetForestGuard(c.ForestGuard)

					selector := &anise_pkg.DefaultPackage{
						Name:     gpkg.Name,
						Category: gpkg.Category,
						Version:  gpkg.Version,
					}
					pack, err := s.ResolvePackage(selector)
					if err != nil {
						return ans, fmt.Errorf("error on resolve package %s: %s",
							selector.HumanReadableString(), err.Error())
					}

					rpackage := pack.Artifacts[0].GetPackage()

					if c.Specs.List.ToIgnore(rpackage) {
						Debug("Ignoring package %s", p)
						continue
					} else {
						ans = append(ans, rpackage)
					}
				}

			}

		}

	}

	return ans, nil
}

func (c *RepoList) sortPkgsThinArr4Availables(refarr *[]*pkg.PackageThin) error {

	ans := []*pkg.PackageThin{}
	pinject := make(map[string]bool, 0)
	queue := make(map[string]*pkg.PackageThin, 0)

	mapPkgs := make(map[string]bool, 0)

	// Setup mapPkgs with the PackageName as key
	for _, p := range *refarr {
		mapPkgs[p.PackageName()] = true
	}

	anise_pkg.SortPackageThinList4Requires(refarr)

	start := time.Now()
	for _, p := range *refarr {

		injected := false

		if !p.HasRequires() {
			ans = append(ans, p)
			pinject[p.PackageName()] = true
			injected = true

		} else {
			allReqok := true

			for _, r := range p.Requires {

				// Check if the requires is present is the selected map and
				// skip the dependency if not selected.
				if _, present := mapPkgs[r.PackageName()]; !present {
					continue
				}

				if _, ok := pinject[r.PackageName()]; !ok {
					allReqok = false
					break
				}
			}

			if allReqok {
				ans = append(ans, p)
				pinject[p.PackageName()] = true
				injected = true

			} else {
				queue[p.PackageName()] = p
			}

		}

		if injected {
			// POST: check if the elements in queue
			//       could be injected.

			pkgs2remove := []string{}
			for k, pr := range queue {

				allReqok := true
				for _, r := range pr.Requires {

					// Check if the requires is present is the selected map and
					// skip the dependency if not selected.
					if _, present := mapPkgs[r.PackageName()]; !present {
						continue
					}

					if _, ok := pinject[r.PackageName()]; !ok {
						allReqok = false
						break
					}
				}

				if allReqok {
					ans = append(ans, pr)
					pinject[pr.PackageName()] = true
					pkgs2remove = append(pkgs2remove, k)
				}

			}

			for _, rm := range pkgs2remove {
				delete(queue, rm)
			}
		}

	} // end for

	Debug(fmt.Sprintf("First sort iteration done in %d µs. Queue size is %d",
		time.Now().Sub(start).Nanoseconds()/1e3, len(queue)))

	if len(queue) > 0 {
		// TODO: review with a more optimized logic

		loopCyclesDetectorCounter := 0
		lastQueueSize := 0
		detectorExecuted := false

		for len(queue) > 0 {
			if len(queue) == lastQueueSize {
				if loopCyclesDetectorCounter > 3 {
					if detectorExecuted {
						return fmt.Errorf(
							"Unexpected error on sort queue of size %d",
							len(queue))
					}
					detectorExecuted = true

					err := c.depCycleDetector(&queue, &pinject, &mapPkgs)
					if err != nil {
						return err
					}

				} else {
					loopCyclesDetectorCounter++
				}
			} else {
				lastQueueSize = len(queue)
				loopCyclesDetectorCounter = 0
			}

			pkgs2remove := []string{}
			for k, p := range queue {

				allReqok := true
				for _, r := range p.Requires {

					// Check if the requires is present is the selected map and
					// skip the dependency if not selected.
					if _, present := mapPkgs[r.PackageName()]; !present {
						continue
					}

					if _, ok := pinject[r.PackageName()]; !ok {
						// Check if there are dependency cycles.
						// I prefer leave this check only here to reduce
						// the impact of a wrong order.
						rr, _ := queue[r.PackageName()]
						if rr != nil && rr.RequirePackage(p) {
							Warning(fmt.Sprintf(
								"Found dependency cycle between %s and %s. I break cycle. Check the repo.",
								p.HumanReadableString(), r.HumanReadableString()))
							allReqok = true
						} else {
							Debug(fmt.Sprintf("[%s] The dependency %s has deps: %s",
								p.HumanReadableString(), r.HumanReadableString(), r.Requires))
							allReqok = false
						}
						break
					} else {
						Debug(fmt.Sprintf("[%s] Dependency %s already injected.", p.HumanReadableString(),
							r.PackageName()))
					}
				}

				if allReqok {
					ans = append(ans, p)
					pinject[p.PackageName()] = true
					pkgs2remove = append(pkgs2remove, k)
				}

			}

			for _, rm := range pkgs2remove {
				delete(queue, rm)
			}

		}
	}

	*refarr = ans

	return nil
}

func (c *RepoList) depCycleDetector(queueRef *map[string]*pkg.PackageThin,
	pinjectRef *map[string]bool, mapPkgsRef *map[string]bool) error {

	elems := []*pkg.PackageThin{}
	queue := *queueRef
	pinject := *pinjectRef
	mapPkgs := *mapPkgsRef

	// Convert map to list
	for _, p := range queue {
		elems = append(elems, p)
	}

	// Sort elements list
	pkg.SortPackageThinList4Requires(&elems)

	nelems := len(elems)
	for idx, p := range elems {
		stack := []*pkg.PackageThin{}
		Debug(fmt.Sprintf(
			"[%d of %d] Checking %s with %d requires...", idx+1, nelems, p.HumanReadableString(),
			len(p.Requires)))

		err := c.recursiveCheckDeps(p, &stack, queueRef, pinjectRef, mapPkgsRef)
		if err != nil {
			return err
		}

		// Check that all dependencies are availables.
		for _, r := range p.Requires {

			// Check if the requires is present is the selected map and
			// skip the dependency if not selected.
			if _, present := mapPkgs[r.PackageName()]; !present {
				continue
			}

			_, injected := pinject[r.PackageName()]
			_, inQueue := queue[r.PackageName()]

			if !injected && !inQueue {
				return fmt.Errorf("[%s] Found dependency not resolvable %s",
					p.PackageName(), r.HumanReadableString())
			}
		}
	}

	return nil
}

func (c *RepoList) recursiveCheckDeps(p *pkg.PackageThin,
	stackRef *[]*pkg.PackageThin, queueRef *map[string]*pkg.PackageThin,
	pinjectRef *map[string]bool, mapPkgsRef *map[string]bool) error {

	Debug(fmt.Sprintf("[%s] with stack %s and requires %d", p, *stackRef, len(p.Requires)))
	if pkg.PackageThinIsInList(p, stackRef) {
		Warning(fmt.Sprintf(
			":ambulance: Found deps cycle for package %s: %s. Trying to break it. Please, fix specs or open an issue!!!",
			p.PackageName(), *stackRef))

		p.BreakCyclesOnRequires(stackRef)
	}
	*stackRef = append(*stackRef, p)
	queue := *queueRef
	pinject := *pinjectRef
	mapPkgs := *mapPkgsRef

	for _, r := range p.Requires {

		// Check if the requires is present is the selected map and
		// skip the dependency if not selected.
		if _, present := mapPkgs[r.PackageName()]; !present {
			continue
		}

		// The requires of the package doesn't contain the dependencies.
		// I need to use the queue element.

		rq, ok := queue[r.PackageName()]
		if !ok {
			// Check if the dependencies is already injected
			_, injected := pinject[r.PackageName()]

			if !injected {
				return fmt.Errorf("[%s] Error on retrieve requires %s on queue",
					p.HumanReadableString(), r.PackageName())
			}
			// POST: Ignoring this dependency
			Debug(fmt.Sprintf("[%s] Dependency %s already injected. Ignoring.",
				p.HumanReadableString(), r.HumanReadableString()))
			continue
		}

		err := c.recursiveCheckDeps(rq, stackRef, queueRef, pinjectRef, mapPkgsRef)
		if err != nil {
			return err
		}
	}

	return nil

}

func (c *RepoList) ListPkgsMissingByDeps(treePaths []string, withResolve bool) ([]*anise_pkg.DefaultPackage, error) {
	ans := []*anise_pkg.DefaultPackage{}

	list, err := c.ListPkgsMissing()
	if err != nil {
		return list, err
	}

	if len(list) == 0 {
		// POST: Nothing to do
		return ans, nil
	}

	// NOTE: At this level we don't need to resolve completely the dependencies
	//       because the sort of the missing packages need to be just in a right order.
	//       Missed dependencies from other tree could be ignored and skipped.

	// Create a map of the packages
	mMissings := make(map[string]*anise_pkg.DefaultPackage, 0)
	// Create the package thin array
	pthinarr := []*pkg.PackageThin{}

	for idx, p := range list {
		mMissings[p.HumanReadableString()] = list[idx]
		pthinarr = append(pthinarr, p.ToPackageThin())
	}

	if withResolve {
		err = c.sortPkgsThinArr4Availables(&pthinarr)
		if err != nil {
			return nil, err
		}
	}

	for idx := range pthinarr {
		if p, present := mMissings[pthinarr[idx].HumanReadableString()]; present {
			ans = append(ans, p)
		}
	}

	return ans, nil
}
