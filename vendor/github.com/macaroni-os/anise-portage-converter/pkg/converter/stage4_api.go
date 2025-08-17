/*
Copyright © 2021-2023 Macaroni OS Linux
See AUTHORS and LICENSE for the license details and contributors.
*/
package converter

import (
	"errors"
	"fmt"

	cfg "github.com/geaaru/luet/pkg/config"
	. "github.com/geaaru/luet/pkg/logger"
	luet_pkg "github.com/geaaru/luet/pkg/package"
)

type Stage4Leaf struct {
	Package  *luet_pkg.DefaultPackage
	Father   []*luet_pkg.DefaultPackage
	Position int
	Counter  int
}

type Stage4Tree struct {
	Id   int
	Map  map[string]*Stage4Leaf
	Deps []*luet_pkg.DefaultPackage
}

type Stage4LeafCache struct {
	MapAssign map[string]*luet_pkg.DefaultPackage
}

type Stage4Levels struct {
	Name      string
	Levels    []*Stage4Tree
	Map       map[string]*luet_pkg.DefaultPackage
	Changed   map[string]*luet_pkg.DefaultPackage
	CacheLeaf map[string]*Stage4LeafCache

	Quiet bool
}

func NewStage4LeafCache() *Stage4LeafCache {
	return &Stage4LeafCache{
		MapAssign: make(map[string]*luet_pkg.DefaultPackage, 0),
	}
}

func NewStage4Levels() *Stage4Levels {
	return &Stage4Levels{
		Levels:    []*Stage4Tree{},
		Map:       make(map[string]*luet_pkg.DefaultPackage, 0),
		Changed:   make(map[string]*luet_pkg.DefaultPackage, 0),
		CacheLeaf: make(map[string]*Stage4LeafCache, 0),
		Quiet:     false,
	}
}

func NewStage4LevelsWithSize(nLevels int) *Stage4Levels {
	ans := NewStage4Levels()
	for i := 0; i < nLevels; i++ {
		tree := NewStage4Tree(i + 1)
		ans.AddTree(tree)
	}

	return ans
}

func (l *Stage4Levels) PackageHasAncient(pkg, ancient *luet_pkg.DefaultPackage, startLevel int) (bool, error) {
	ans := false
	found := false
	key := fmt.Sprintf("%s/%s", pkg.GetCategory(), pkg.GetName())

	if startLevel >= len(l.Levels) {
		return false, errors.New("Invalid start level")
	}

	lastLevel := len(l.Levels) - 1
	for i := startLevel; i <= lastLevel; i++ {
		// Check if the package is present on the level.
		if _, ok := l.Levels[i].Map[key]; !ok {
			continue
		}
		found = true

		for _, dep := range l.Levels[i].Map[key].Package.GetRequires() {
			if dep.GetName() == ancient.GetName() &&
				dep.GetCategory() == ancient.GetCategory() {
				ans = true
				break
			}
			if i < lastLevel {
				hasAncient, err := l.PackageHasAncient(dep, ancient, i+1)
				if err != nil {
					return ans, err
				}

				if hasAncient {
					ans = true
					break
				}
			}
		}

		if ans {
			break
		}
	}

	if !found {
		return false, errors.New("Package " + key + " not found on level " + fmt.Sprintf("%d", startLevel))
	}

	return ans, nil
}

func (l *Stage4Levels) GetMap() *map[string]*luet_pkg.DefaultPackage { return &l.Map }

func (l *Stage4Levels) Dump() string {
	ans := ""
	for _, t := range l.Levels {
		ans += t.Dump()
	}

	return ans
}

func (l *Stage4Levels) AddTree(t *Stage4Tree) {
	l.Levels = append(l.Levels, t)
}

func (l *Stage4Levels) RegisterAssignment(p, father, newFather *luet_pkg.DefaultPackage) error {

	key := fmt.Sprintf("%s/%s", p.GetCategory(), p.GetName())
	fatherKey := fmt.Sprintf("%s/%s", father.GetCategory(), father.GetName())

	v, ok := l.CacheLeaf[key]

	if !ok {
		v = NewStage4LeafCache()
	}

	v.MapAssign[fatherKey] = newFather
	l.CacheLeaf[key] = v

	return nil
}

func (l *Stage4Levels) GetAssignment(p, father *luet_pkg.DefaultPackage) *luet_pkg.DefaultPackage {
	key := fmt.Sprintf("%s/%s", p.GetCategory(), p.GetName())
	fatherKey := fmt.Sprintf("%s/%s", father.GetCategory(), father.GetName())

	v, ok := l.CacheLeaf[key]

	if !ok {
		return nil
	}

	ans, _ := v.MapAssign[fatherKey]
	return ans
}

func (l *Stage4Levels) AddDependency(p, father *luet_pkg.DefaultPackage, level int) error {
	if level >= len(l.Levels) {
		return errors.New("Invalid level")
	}

	key := fmt.Sprintf("%s/%s", p.GetCategory(), p.GetName())

	v, ok := l.Map[key]
	if ok {
		l.Levels[level].AddDependency(v, father)
	} else {
		l.Map[key] = p
		l.Levels[level].AddDependency(p, father)
	}

	return nil
}

func IsInStack(stack []string, pkg string) bool {
	ans := false
	for _, p := range stack {
		if p == pkg {
			ans = true
			break
		}
	}
	return ans
}

func (l *Stage4Levels) AddDependencyRecursive(p, father *luet_pkg.DefaultPackage, stack []string, level int) (bool, error) {
	var err error
	var toInsert bool = true

	if level >= len(l.Levels) {
		return false, errors.New("Invalid level")
	}

	key := fmt.Sprintf("%s/%s", p.GetCategory(), p.GetName())

	if cfg.LuetCfg.GetLogging().Level == "debug" {
		DebugC(fmt.Sprintf("Adding recursive %s package to level %d (%v)", key, level+1, stack))
	}

	_, ok := l.Map[key]
	if !ok {
		return false, errors.New(fmt.Sprintf("On add dependency not found package %s/%s",
			p.GetCategory(), p.GetName()))
	}

	inStack := IsInStack(stack, key)
	if inStack {
		return true, nil
	}

	stack = append(stack, key)

	if len(p.GetRequires()) > 0 {

		for _, d := range p.GetRequires() {

			key = fmt.Sprintf("%s/%s", d.GetCategory(), d.GetName())
			v, ok := l.Map[key]
			if !ok {
				return false, errors.New(fmt.Sprintf("For package %s/%s not found dependency %s",
					p.GetCategory(), p.GetName(), key))
			}

			toInsert, err = l.AddDependencyRecursive(v, p, stack, level+1)
			if err != nil {
				return false, err
			}

			if !toInsert {
				break
			}
		}
	}

	if toInsert {
		return toInsert, l.Levels[level].AddDependency(p, father)
	} else {
		DebugC(GetAurora().Bold(fmt.Sprintf("For package %s break cycle.", key)))
		return false, nil
	}
}

func (l *Stage4Levels) AddChangedPackage(pkg *luet_pkg.DefaultPackage) {
	key := fmt.Sprintf("%s/%s", pkg.GetCategory(), pkg.GetName())
	l.Changed[key] = pkg
}

func (l *Stage4Levels) analyzeLevelLeafs(pos int) (bool, error) {
	tree := l.Levels[pos]
	rescan := false

	if cfg.LuetCfg.GetLogging().Level == "debug" {
		// Call this only with debug enabled.
		DebugC(fmt.Sprintf("[%d-%d] Tree:\n%s", tree.Id, pos, tree.Dump()))
	}

	for _, leaf := range *tree.GetMap() {

		r, err := l.AnalyzeLeaf(pos, tree, leaf)
		if err != nil {
			return rescan, err
		}

		if r {
			rescan = true
			break
		}
	} // end for key map

	return rescan, nil
}

func (l *Stage4Levels) Resolve() error {
	// Start from bottom
	pos := len(l.Levels)

	// Check if the levels are sufficient for serialization
	missingLevels := len(l.Levels[0].Map) - pos
	// POST: we need to add levels.
	for i := 1; i <= missingLevels; i++ {
		tree := NewStage4Tree(pos + i)
		l.AddTree(tree)
	}
	initialLevels := len(l.Levels)

	tot_pkg := len(l.Levels[0].Map)

	for pos > 0 {
		pos--

		pkgs := tot_pkg - len(l.Levels[0].Map)

		rescan, err := l.analyzeLevelLeafs(pos)
		if err != nil {
			return err
		}
		if rescan {
			if !l.Quiet {
				InfoC(
					fmt.Sprintf(
						"%s Analyzed packages %2d/%2d ...",
						l.Name, pkgs, tot_pkg,
					))
			}
			// Restarting analysis from begin
			pos = initialLevels
		}
	}

	return nil
}

func NewStage4Leaf(pkg, father *luet_pkg.DefaultPackage, pos int) (*Stage4Leaf, error) {
	if pkg == nil || pos < 0 {
		return nil, errors.New("Invalid parameter on create stage4 leaf")
	}

	ans := &Stage4Leaf{
		Package:  pkg,
		Father:   []*luet_pkg.DefaultPackage{},
		Position: pos,
		Counter:  1,
	}

	if father != nil {
		ans.Father = append(ans.Father, father)
	}

	return ans, nil
}

func (l *Stage4Leaf) String() string {

	ans := fmt.Sprintf("%s/%s (%d, %d) ",
		l.Package.GetCategory(), l.Package.GetName(),
		l.Position, l.Counter)

	if len(l.Father) > 0 {
		ans += " father: ["
		for _, f := range l.Father {
			ans += fmt.Sprintf("%s/%s, ", f.GetCategory(), f.GetName())
		}
		ans += "]"
	}

	return ans
}

func (l *Stage4Leaf) AddFather(father *luet_pkg.DefaultPackage) {

	// Check if the father is already present
	notFound := true

	for _, f := range l.Father {
		if f.GetCategory() == father.GetCategory() &&
			f.GetName() == father.GetName() {
			notFound = false
			break
		}
	}

	if notFound {
		l.Father = append(l.Father, father)
		l.Counter++
	}
}

func (l *Stage4Leaf) DelFather(father *luet_pkg.DefaultPackage) {
	pos := -1
	for idx, f := range l.Father {
		if f.GetCategory() == father.GetCategory() &&
			f.GetName() == father.GetName() {
			pos = idx
			break
		}
	}

	if pos >= 0 {
		l.Father = append(l.Father[:pos], l.Father[pos+1:]...)
		l.Counter--

		DebugC(fmt.Sprintf("From leaf %s/%s delete father %s/%s (%d)",
			l.Package.GetCategory(), l.Package.GetName(),
			father.GetCategory(), father.GetName(), l.Counter,
		))
	}
}

func NewStage4Tree(id int) *Stage4Tree {
	return &Stage4Tree{
		Id:   id,
		Map:  make(map[string]*Stage4Leaf, 0),
		Deps: []*luet_pkg.DefaultPackage{},
	}
}

func (t *Stage4Tree) Dump() string {
	ans := fmt.Sprintf("[%d] Map: [ ", t.Id)

	for k, v := range t.Map {
		ans += fmt.Sprintf("%s-%d ", k, v.Counter)

		if v.Counter > 0 {
			ans += "("
			for _, father := range v.Father {
				ans += fmt.Sprintf("%s/%s, ", father.GetCategory(), father.GetName())
			}
			ans += ")"
		}

		ans += ", "
	}
	ans += "]\n"
	ans += fmt.Sprintf("[%d] Deps: [ ", t.Id)
	for _, d := range t.Deps {
		ans += fmt.Sprintf("%s/%s", d.GetCategory(), d.GetName())

		if len(d.GetRequires()) > 0 {
			ans += " ("
			for _, d2 := range d.GetRequires() {
				ans += fmt.Sprintf("%s/%s, ", d2.GetCategory(), d2.GetName())
			}
			ans += " )"
		}
		ans += ", "

	}
	ans += "]\n"
	return ans
}

func (t *Stage4Tree) GetMap() *map[string]*Stage4Leaf {
	return &t.Map
}

func (t *Stage4Tree) GetDeps() *[]*luet_pkg.DefaultPackage { return &t.Deps }

func (t *Stage4Tree) GetDependency(pos int) (*luet_pkg.DefaultPackage, error) {
	if len(t.Deps) <= pos {
		return nil, errors.New("Invalid position")
	}

	return t.Deps[pos], nil
}

func (t *Stage4Tree) DropDependency(p *luet_pkg.DefaultPackage) error {
	key := fmt.Sprintf("%s/%s", p.GetCategory(), p.GetName())
	leaf, ok := t.Map[key]
	if !ok {
		return errors.New(fmt.Sprintf("Package %s is not present on tree.", key))
	}

	if leaf.Counter > 1 {
		leaf.Counter--
		DebugC(fmt.Sprintf("For %s decrement counter.", key))
		return nil
	}

	DebugC(fmt.Sprintf("[%d] Dropping dependency %s...", t.Id, key), len(t.Map))

	delete(t.Map, key)

	if len(t.Deps) == (leaf.Position - 1) {
		// POST: just drop last element
		t.Deps = t.Deps[:leaf.Position-1]

	} else if leaf.Position == 0 {
		// POST: just drop the first element
		t.Deps = t.Deps[1:]

	} else {
		// POST: drop element between other elements
		t.Deps = append(t.Deps[:leaf.Position], t.Deps[leaf.Position+1:]...)
	}

	for k, v := range t.Map {
		if v.Position > leaf.Position {
			t.Map[k].Position--
		}
	}

	DebugC(fmt.Sprintf("[%d] Dropped dependency %s...", t.Id, key), len(t.Map))

	return nil
}

func (t *Stage4Tree) AddDependency(p, father *luet_pkg.DefaultPackage) error {
	fatherKey := "-"
	if father != nil {
		fatherKey = fmt.Sprintf("%s/%s", father.GetCategory(), father.GetName())
	}
	DebugC(fmt.Sprintf("[L%d] Adding dep %s/%s with father %s",
		t.Id, p.GetCategory(), p.GetName(), fatherKey))

	if leaf, ok := t.Map[fmt.Sprintf("%s/%s", p.GetCategory(), p.GetName())]; ok {
		leaf.AddFather(father)
		return nil
	}

	leaf, err := NewStage4Leaf(p, father, len(t.Deps))
	if err != nil {
		return err
	}

	t.Map[fmt.Sprintf("%s/%s", p.GetCategory(), p.GetName())] = leaf
	t.Deps = append(t.Deps, p)

	return nil
}

// TODO: Move in another place
func RemoveDependencyFromLuetPackage(pkg, dep *luet_pkg.DefaultPackage) error {

	DebugC(GetAurora().Bold(fmt.Sprintf("Removing dep %s/%s from package %s/%s...",
		dep.GetCategory(), dep.GetName(),
		pkg.GetCategory(), pkg.GetName(),
	)))

	pos := -1
	for idx, d := range pkg.GetRequires() {
		if d.GetCategory() == dep.GetCategory() && d.GetName() == dep.GetName() {
			pos = idx
			break
		}
	}

	if pos < 0 {
		// Ignore error until we fix father cleanup
		Warning(fmt.Sprintf("Dependency %s/%s not found on package %s/%s",
			dep.GetCategory(), dep.GetName(),
			pkg.GetCategory(), pkg.GetName(),
		))
		/*
			return nil
		*/
		return errors.New(
			fmt.Sprintf("Dependency %s/%s not found on package %s/%s",
				dep.GetCategory(), dep.GetName(),
				pkg.GetCategory(), pkg.GetName(),
			))
	}

	pkg.PackageRequires = append(pkg.PackageRequires[:pos], pkg.PackageRequires[pos+1:]...)

	return nil
}

func AddDependencyToLuetPackage(pkg, dep *luet_pkg.DefaultPackage) {

	DebugC(fmt.Sprintf("Adding %s/%s dep to package %s/%s",
		dep.GetCategory(), dep.GetName(),
		pkg.GetCategory(), pkg.GetName()))

	// Check if dependency is already present.
	notFound := true
	for _, d := range pkg.GetRequires() {
		if d.GetCategory() == dep.GetCategory() && d.GetName() == dep.GetName() {
			notFound = false
			break
		}
	}
	if notFound {
		DebugC(GetAurora().Bold(fmt.Sprintf("Added %s/%s dep to package %s/%s",
			dep.GetCategory(), dep.GetName(),
			pkg.GetCategory(), pkg.GetName())))

		// TODO: check if we need set PackageRequires
		pkg.PackageRequires = append(pkg.PackageRequires,
			&luet_pkg.DefaultPackage{
				Category: dep.GetCategory(),
				Name:     dep.GetName(),
				Version:  ">=0",
			},
		)
	}

}
