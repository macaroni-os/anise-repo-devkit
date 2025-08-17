/*
Copyright © 2021-2023 Macaroni OS Linux
See AUTHORS and LICENSE for the license details and contributors.
*/
package converter

import (
	"errors"
	"fmt"

	//cfg "github.com/geaaru/luet/pkg/config"
	. "github.com/geaaru/luet/pkg/logger"
	luet_pkg "github.com/geaaru/luet/pkg/package"
)

func (l *Stage4Levels) AnalyzeLeaf(pos int, tree *Stage4Tree, leaf *Stage4Leaf) (bool, error) {
	var firstFatherLeaf *luet_pkg.DefaultPackage = nil
	var lastFatherLeaf *luet_pkg.DefaultPackage = nil
	var fathersHandled map[string]*luet_pkg.DefaultPackage = make(map[string]*luet_pkg.DefaultPackage, 0)
	rescan := false
	isLowerLevel := false
	nextLevel := pos - 1

	DebugC(GetAurora().Bold(fmt.Sprintf(
		"[P%d-L%d] Levels:\n%s", pos, tree.Id, l.Dump()),
	))

	key := fmt.Sprintf("%s/%s", leaf.Package.GetCategory(), leaf.Package.GetName())

	if pos == len(l.Levels)-1 {
		isLowerLevel = true
	}

	DebugC(fmt.Sprintf("[P%d-L%d] Processing leaf %s - lower lever = %v",
		pos, tree.Id, leaf, isLowerLevel))

	if len(leaf.Father) == 0 && pos != 0 {
		return false, errors.New(fmt.Sprintf(
			"Unexpected leaf without father at level %d for package %s",
			pos, key))
	}

	// Manage leaf of the elaborated level. I identify the first father from
	// this level.

	if len(leaf.Father) > 0 {
		lastFatherLeaf = leaf.Father[0]
	}

	if leaf.Counter > 1 {
		// POST: we have different packages
		//       with the same dependency

		if len(leaf.Father) > 0 {
			toRemove := []*luet_pkg.DefaultPackage{}
			for idx, _ := range leaf.Father {
				if idx == 0 {
					firstFatherLeaf = leaf.Father[idx]

					continue
				}
				// The father must to point at the father of the last leaf.
				// Removing the analyzed dependency from the father.
				err := RemoveDependencyFromLuetPackage(
					leaf.Father[idx],
					leaf.Package)
				if err != nil {
					return false, err
				}

				keyFather := fmt.Sprintf(leaf.Father[idx].GetCategory(), leaf.Father[idx].GetName())
				toAdd, err := l.AddDependencyRecursive(leaf.Father[idx-1], leaf.Father[idx], []string{}, pos)
				if err != nil {
					return false, err
				}

				if toAdd {
					// POST Add the dependency only if there isn't a cycle.
					AddDependencyToLuetPackage(leaf.Father[idx], leaf.Father[idx-1])
					lastFatherLeaf = leaf.Father[idx]
					fathersHandled[keyFather] = leaf.Father[idx-1]
					rescan = true
				}

				l.AddChangedPackage(leaf.Father[idx])

				toRemove = append(toRemove, leaf.Father[idx])

			}

			if len(toRemove) > 0 {
				for _, f := range toRemove {
					leaf.DelFather(f)
				}
			}
		}

	} else if len(leaf.Father) > 0 {
		firstFatherLeaf = leaf.Father[0]
	}

	for nextLevel >= 0 {
		DebugC(fmt.Sprintf("[P%d-L%d] Analyze upper levels for leaf %s (%d)",
			pos, tree.Id, key, nextLevel))

		treeUpper := l.Levels[nextLevel]

		l2, ok := treeUpper.Map[key]
		if ok {

			// POST: found the package with the selected key
			if len(l2.Father) == 0 {
				DebugC(fmt.Sprintf("For %s father is nil.", key))
				if nextLevel == 0 {
					treeUpper.DropDependency(leaf.Package)
				}
			} else {

				toRemove := []*luet_pkg.DefaultPackage{}

				for idx, _ := range l2.Father {

					DebugC(fmt.Sprintf("[L%d] For %s analyze father %s (%v)",
						treeUpper.Id, key, l2.Father[idx], isLowerLevel))

					if firstFatherLeaf != nil &&
						l2.Father[idx].GetCategory() == firstFatherLeaf.GetCategory() &&
						l2.Father[idx].GetName() == firstFatherLeaf.GetName() {

						DebugC(fmt.Sprintf(
							"[L%d] For key %s the father %s/%s is the first father. Nothing to do.",
							treeUpper.Id, key, l2.Father[idx].GetCategory(), l2.Father[idx].GetName()))
						treeUpper.DropDependency(leaf.Package)

					} else if lastFatherLeaf != nil &&
						l2.Father[idx].GetCategory() == lastFatherLeaf.GetCategory() &&
						l2.Father[idx].GetName() == lastFatherLeaf.GetName() {

						DebugC(fmt.Sprintf(
							"[L%d] For key %s the father %s/%s is the last father. Nothing to do.",
							treeUpper.Id, key, l2.Father[idx].GetCategory(), l2.Father[idx].GetName()))

					} else {

						fatherKey := fmt.Sprintf(l2.Father[idx].GetCategory(), l2.Father[idx].GetName())
						_, pkgAlreadyElaborated := fathersHandled[fatherKey]

						if !pkgAlreadyElaborated {
							// POST: The father must to point at the father of the last leaf.

							err := RemoveDependencyFromLuetPackage(
								l2.Father[idx],
								leaf.Package)
							if err != nil {
								return false, err
							}

							toAdd, err := l.AddDependencyRecursive(lastFatherLeaf, l2.Father[idx], []string{}, nextLevel)
							if err != nil {
								return false, err
							}
							if toAdd {
								AddDependencyToLuetPackage(l2.Father[idx], lastFatherLeaf)
								fathersHandled[fatherKey] = lastFatherLeaf
								lastFatherLeaf = l2.Father[idx]
								toRemove = append(toRemove, l2.Father[idx])
								rescan = true
							}

							l.AddChangedPackage(l2.Father[idx])

						} else {
							DebugC(fmt.Sprintf("For %s nothing to do. Father %s/%s is with deps: %s",
								key, l2.Father[idx].GetCategory(), l2.Father[idx].GetName(),
								l2.Father[idx].GetRequires()))

							toRemove = append(toRemove, l2.Father[idx])
						} ///

					}

				} // end for

				if len(toRemove) > 0 {
					for _, f := range toRemove {
						l2.DelFather(f)
					}
				}

				if nextLevel > 0 {
					// Remove the package from the tree.
					treeUpper.DropDependency(leaf.Package)
				}
			}

		}

		DebugC(GetAurora().Bold(fmt.Sprintf(
			"[P%d-L%d] Completed analysis of the level %d for leaf %s: key found: %v (lasfFather = %s/%s)",
			pos, tree.Id, treeUpper.Id, key, ok, lastFatherLeaf.GetCategory(),
			lastFatherLeaf.GetName())))

		nextLevel--
	}

	return rescan, nil
}
