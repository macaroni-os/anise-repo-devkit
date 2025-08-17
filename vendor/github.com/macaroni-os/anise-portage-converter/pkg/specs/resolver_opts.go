/*
Copyright © 2021-2023 Macaroni OS Linux
See AUTHORS and LICENSE for the license details and contributors.
*/
package specs

func NewPortageResolverOpts() *PortageResolverOpts {
	return &PortageResolverOpts{
		EnableUseFlags:   []string{},
		DisabledUseFlags: []string{},
		Conditions:       []string{},
	}
}

func (o *PortageResolverOpts) IsAdmitUseFlag(u string) bool {
	ans := true
	if len(o.EnableUseFlags) > 0 {
		for _, ue := range o.EnableUseFlags {
			if ue == u {
				return true
			}
		}

		return false
	}

	if len(o.DisabledUseFlags) > 0 {
		for _, ud := range o.DisabledUseFlags {
			if ud == u {
				ans = false
				break
			}
		}
	}

	return ans
}
