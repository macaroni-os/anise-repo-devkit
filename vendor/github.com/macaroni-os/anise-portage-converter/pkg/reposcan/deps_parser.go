/*
Copyright © 2021-2023 Macaroni OS Linux
See AUTHORS and LICENSE for the license details and contributors.
*/
package reposcan

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	. "github.com/geaaru/luet/pkg/logger"
	_gentoo "github.com/geaaru/pkgs-checker/pkg/gentoo"
)

type GentooDependency struct {
	Use          string
	UseCondition _gentoo.PackageCond
	SubDeps      []*GentooDependency
	Dep          *_gentoo.GentooPackage
	DepInOr      bool
}

type EbuildDependencies struct {
	Dependencies []*GentooDependency
}

func NewGentooDependency(pkg, use string) (*GentooDependency, error) {
	var err error
	ans := &GentooDependency{
		Use:     use,
		SubDeps: make([]*GentooDependency, 0),
	}

	if strings.HasPrefix(use, "!") {
		ans.Use = ans.Use[1:]
		ans.UseCondition = _gentoo.PkgCondNot
	}

	if pkg != "" {
		ans.Dep, err = _gentoo.ParsePackageStr(pkg)
		if err != nil {
			return nil, err
		}

		// TODO: Fix support of slot with :=
		//       Drop it for now.
		ans.Dep.Slot = strings.ReplaceAll(ans.Dep.Slot, "=", "")

		// TODO: Fix this on parsing phase for handle correctly ${PV}
		if strings.HasSuffix(ans.Dep.Name, "-") {
			ans.Dep.Name = ans.Dep.Name[:len(ans.Dep.Name)-1]
		}

	}

	return ans, nil
}

func NewGentooDependencyWithSubdeps(pkg, use string, subdeps []*GentooDependency) (*GentooDependency, error) {
	ans, err := NewGentooDependency(pkg, use)
	if err != nil {
		return ans, err
	}

	ans.SubDeps = subdeps
	return ans, nil
}

func (d *GentooDependency) String() string {
	if d.Dep != nil {
		return fmt.Sprintf("%s (%v)", d.Dep, d.DepInOr)
	} else {
		return fmt.Sprintf("%s %d %s (%v)", d.Use, d.UseCondition, d.SubDeps, d.DepInOr)
	}
}

func (d *GentooDependency) GetDepsList() []*GentooDependency {
	ans := make([]*GentooDependency, 0)

	if len(d.SubDeps) > 0 {
		for _, d2 := range d.SubDeps {
			list := d2.GetDepsList()
			ans = append(ans, list...)
		}
	}

	if d.Dep != nil {
		ans = append(ans, d)
	}

	return ans
}

func (d *GentooDependency) GetUseFlags() []string {
	ans := []string{}

	if d.Use != "" {
		ans = append(ans, d.Use)
	}

	if len(d.SubDeps) > 0 {
		for _, sd := range d.SubDeps {
			ul := sd.GetUseFlags()
			if len(ul) > 0 {
				ans = append(ans, ul...)
			}
		}
	}

	return ans
}

func (d *GentooDependency) AddSubDependency(pkg, use string) (*GentooDependency, error) {
	ans, err := NewGentooDependency(pkg, use)
	if err != nil {
		return nil, err
	}

	d.SubDeps = append(d.SubDeps, ans)

	return ans, nil
}

func (r *EbuildDependencies) GetDependencies() []*GentooDependency {
	ans := make([]*GentooDependency, 0)

	for _, d := range r.Dependencies {
		list := d.GetDepsList()
		ans = append(ans, list...)
	}

	// the same dependency could be available in multiple use flags.
	// It's needed avoid duplicate.
	m := make(map[string]*GentooDependency, 0)

	for _, p := range ans {
		m[p.String()] = p
	}

	ans = make([]*GentooDependency, 0)
	for _, p := range m {
		ans = append(ans, p)
	}

	return ans
}

func (r *EbuildDependencies) GetUseFlags() []string {
	ans := []string{}

	for _, d := range r.Dependencies {
		ul := d.GetUseFlags()
		if len(ul) > 0 {
			ans = append(ans, ul...)
		}
	}

	// Drop duplicate
	m := make(map[string]int, 0)
	for _, u := range ans {
		m[u] = 1
	}

	ans = []string{}
	for k, _ := range m {
		ans = append(ans, k)
	}

	return ans
}

func ParseDependenciesMultiline(rdepend string) (*EbuildDependencies, error) {
	var lastdep []*GentooDependency = make([]*GentooDependency, 0)
	var pendingDep = false
	var orDep = false
	var dep *GentooDependency
	var err error

	ans := &EbuildDependencies{
		Dependencies: make([]*GentooDependency, 0),
	}

	if rdepend != "" {
		rdepends := strings.Split(rdepend, "\n")
		for _, rr := range rdepends {
			rr = strings.TrimSpace(rr)
			if rr == "" {
				continue
			}

			if strings.HasPrefix(rr, "|| (") {
				orDep = true
				continue
			}

			if orDep {
				rr = strings.TrimSpace(rr)
				if rr == ")" {
					orDep = false
				}
				continue
			}

			if strings.Index(rr, "?") > 0 {
				// use flag present

				if pendingDep {
					dep, err = lastdep[len(lastdep)-1].AddSubDependency("", rr[:strings.Index(rr, "?")])
					if err != nil {
						// Debug
						Debug("Ignoring subdependency ", rr[:strings.Index(rr, "?")])
					}
				} else {
					dep, err = NewGentooDependency("", rr[:strings.Index(rr, "?")])
					if err != nil {
						// Debug
						Debug("Ignoring dep", rr)
					} else {
						ans.Dependencies = append(ans.Dependencies, dep)
					}
				}

				if strings.Index(rr, ")") < 0 {
					pendingDep = true
					lastdep = append(lastdep, dep)
				}

				if strings.Index(rr, "|| (") >= 0 {
					// Ignore dep in or
					continue
				}

				fields := strings.Split(rr[strings.Index(rr, "?")+1:], " ")
				for _, f := range fields {
					f = strings.TrimSpace(f)
					if f == ")" || f == "(" || f == "" {
						continue
					}

					_, err = dep.AddSubDependency(f, "")
					if err != nil {
						// Debug
						Debug("Ignoring subdependency ", f)
					}
				}

			} else if pendingDep {
				fields := strings.Split(rr, " ")
				for _, f := range fields {
					f = strings.TrimSpace(f)
					if f == ")" || f == "(" || f == "" {
						continue
					}
					_, err = lastdep[len(lastdep)-1].AddSubDependency(f, "")
					if err != nil {
						return nil, err
					}
				}

				if strings.Index(rr, ")") >= 0 {
					lastdep = lastdep[:len(lastdep)-1]
					if len(lastdep) == 0 {
						pendingDep = false
					}
				}

			} else {
				rr = strings.TrimSpace(rr)
				// Check if there multiple deps in single row

				fields := strings.Split(rr, " ")
				if len(fields) > 1 {
					for _, rrr := range fields {
						rrr = strings.TrimSpace(rrr)
						if rrr == "" {
							continue
						}
						dep, err := NewGentooDependency(rrr, "")
						if err != nil {
							// Debug
							Debug("Ignoring dep", rr)
						} else {
							ans.Dependencies = append(ans.Dependencies, dep)
						}
					}
				} else {
					dep, err := NewGentooDependency(rr, "")
					if err != nil {
						// Debug
						Debug("Ignoring dep", rr)
					} else {
						ans.Dependencies = append(ans.Dependencies, dep)
					}
				}
			}

		}

	}

	return ans, nil
}

func ParseDependencies(rdepend string) (*EbuildDependencies, error) {
	var idx = 0
	var last *GentooDependency
	openParenthesis := 0
	stack := []*GentooDependency{}

	ans := &EbuildDependencies{
		Dependencies: make([]*GentooDependency, 0),
	}

	if rdepend != "" {

		rdepends := strings.Split(rdepend, " ")

		for idx < len(rdepends) {
			rr := rdepends[idx]
			rr = strings.TrimSpace(rr)

			// If there is [...] content i drop it before everything
			reg := regexp.MustCompile(`\[.*\]`)
			rr = reg.ReplaceAllString(rr, "")

			if rr != "" {
				//fmt.Println("PARSING ", rr, openParenthesis)

				if rr == "||" {
					dep, err := NewGentooDependency("", "")
					if err != nil {
						return nil, err
					}
					dep.DepInOr = true

					if last != nil {
						last.SubDeps = append(last.SubDeps, dep)
					} else {
						ans.Dependencies = append(ans.Dependencies, dep)
					}
					stack = append(stack, dep)
					last = dep

					idx++
					continue
				}

				if strings.Index(rr, "?") > 0 {
					// POST: the string is related to use flags
					dep, err := NewGentooDependency("", rr[:strings.Index(rr, "?")])
					if err != nil {
						return nil, err
					} else {
						if last != nil {
							last.SubDeps = append(last.SubDeps, dep)
						} else {
							ans.Dependencies = append(ans.Dependencies, dep)
						}
						stack = append(stack, dep)
						last = dep
					}

					//fmt.Println("Add pendency ", dep)
					idx++
					continue
				}

				if rr == "(" {
					// POST: begin subdeps. Nothing to do.
					if last == nil {
						return nil, errors.New("Unexpected round parenthesis without USE flag")
					}

					if openParenthesis > 0 && last.DepInOr {
						// we need another dep
						dep, err := last.AddSubDependency("", "")
						if err != nil {
							return nil, err
						}
						last = dep

						stack = append(stack, dep)
					}

					openParenthesis++
					idx++
					continue
				}

				if rr == ")" {

					openParenthesis--
					if last == nil {
						return nil, errors.New("Unexpected round parenthesis on empty stack")
					}
					// POST: end subdeps.
					if len(stack) > 1 {

						if stack[len(stack)-2].DepInOr == true && idx < len(rdepends)-2 &&
							strings.TrimSpace(rdepends[idx+1]) != "(" && strings.TrimSpace(rdepends[idx+1]) != ")" &&
							strings.TrimSpace(rdepends[idx+2]) != ")" &&
							// is not a use?
							!strings.Contains(strings.TrimSpace(rdepends[idx+1]), "?") {
							stack = stack[:len(stack)-2]
							if len(stack) > 0 {
								last = stack[len(stack)-1]
							} else {
								last = nil
							}
						} else {
							stack = stack[:len(stack)-1]
							last = stack[len(stack)-1]
						}
					} else {
						stack = []*GentooDependency{}
						last = nil
					}

					idx++
					continue
				}

				if len(stack) > 0 {
					_, err := last.AddSubDependency(rr, "")
					if err != nil {
						return nil, errors.New("Invalid dep " + rr + ": " + err.Error())
					}
				} else {

					dep, err := NewGentooDependency(rr, "")
					if err != nil {
						// Debug
						Debug("Ignoring dep", rr)
					} else {
						ans.Dependencies = append(ans.Dependencies, dep)
					}
				}
			}

			idx++
		}

	}

	return ans, nil
}
