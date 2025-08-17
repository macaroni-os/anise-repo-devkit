/*
Copyright © 2020-2025 Macaroni OS Linux
See AUTHORS and LICENSE for the license details and contributors.
*/
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"

	devkit "github.com/macaroni-os/anise-repo-devkit/pkg/devkit"
	specs "github.com/macaroni-os/anise-repo-devkit/pkg/specs"

	anise_pkg "github.com/geaaru/luet/pkg/package"
	anise_spectooling "github.com/geaaru/luet/pkg/spectooling"
	cobra "github.com/spf13/cobra"
)

func NewPkgsCommand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "pkgs [OPTIONS]",
		Short: "Show packages availables or missings from repository.",
		PreRun: func(cmd *cobra.Command, args []string) {
			treePath, _ := cmd.Flags().GetStringArray("tree")
			listAvailables, _ := cmd.Flags().GetBool("availables")
			listMissings, _ := cmd.Flags().GetBool("missings")

			if len(treePath) == 0 {
				fmt.Println("At least one tree path is needed.")
				os.Exit(1)
			}

			if (listAvailables && listMissings) ||
				(!listAvailables && !listMissings) {
				fmt.Println(
					"It's needed enable or the --availables or --missings options.",
				)
				os.Exit(1)
			}

		},
		Run: func(cmd *cobra.Command, args []string) {
			var s *specs.AniseRDConfig
			var err error

			specsFile, _ := cmd.Flags().GetString("specs-file")
			backend, _ := cmd.Flags().GetString("backend")
			path, _ := cmd.Flags().GetString("path")
			treePath, _ := cmd.Flags().GetStringArray("tree")
			listAvailables, _ := cmd.Flags().GetBool("availables")
			listMissings, _ := cmd.Flags().GetBool("missings")
			buildOrder, _ := cmd.Flags().GetBool("build-ordered")
			buildOrderWithResolve, _ := cmd.Flags().GetBool("build-ordered-with-resolve")
			filters, _ := cmd.Flags().GetStringArray("filter")

			mottainaiProfile, _ := cmd.Flags().GetString("mottainai-profile")
			mottainaiMaster, _ := cmd.Flags().GetString("mottainai-master")
			mottainaiApiKey, _ := cmd.Flags().GetString("mottainai-apikey")
			mottainaiNamespace, _ := cmd.Flags().GetString("mottainai-namespace")

			minioBucket, _ := cmd.Flags().GetString("minio-bucket")
			minioAccessId, _ := cmd.Flags().GetString("minio-keyid")
			minioSecret, _ := cmd.Flags().GetString("minio-secret")
			minioEndpoint, _ := cmd.Flags().GetString("minio-endpoint")
			minioRegion, _ := cmd.Flags().GetString("minio-region")

			jsonOutput, _ := cmd.Flags().GetBool("json")
			limit, _ := cmd.Flags().GetInt32("limit")

			if specsFile == "" {
				s = specs.NewAniseRDConfig()
			} else {
				s, err = specs.LoadSpecsFile(specsFile)
				if err != nil {
					fmt.Println("Error on load specs: " + err.Error())
					os.Exit(1)
				}
			}

			opts := make(map[string]string, 0)
			if backend == "mottainai" {
				if mottainaiProfile != "" {
					opts["mottainai-profile"] = mottainaiProfile
				}
				if mottainaiMaster != "" {
					opts["mottainai-master"] = mottainaiMaster
				}
				if mottainaiApiKey != "" {
					opts["mottainai-apikey"] = mottainaiApiKey
				}
				if mottainaiNamespace != "" {
					opts["mottainai-namespace"] = mottainaiNamespace
				}
			} else if backend == "minio" {

				if minioEndpoint != "" {
					opts["minio-endpoint"] = minioEndpoint
				} else {
					opts["minio-endpoint"] = os.Getenv("MINIO_URL")
				}

				if minioBucket != "" {
					opts["minio-bucket"] = minioBucket
				} else {
					opts["minio-bucket"] = os.Getenv("MINIO_BUCKET")
				}

				if minioAccessId != "" {
					opts["minio-keyid"] = minioAccessId
				} else {
					opts["minio-keyid"] = os.Getenv("MINIO_ID")
				}

				if minioSecret != "" {
					opts["minio-secret"] = minioSecret
				} else {
					opts["minio-secret"] = os.Getenv("MINIO_SECRET")
				}

				opts["minio-region"] = minioRegion

			}

			repoList, err := devkit.NewRepoList(s, backend, path, opts)
			if err != nil {
				fmt.Println("Error on initialize repo list: " + err.Error())
				os.Exit(1)
			}

			// Loading tree in memory
			err = repoList.LoadTrees(treePath)
			if err != nil {
				fmt.Println("Erro on loading trees: " + err.Error())
				os.Exit(1)
			}

			var list []*anise_pkg.DefaultPackage

			if listAvailables {
				list, err = repoList.ListPkgsAvailable()
				if err != nil {
					fmt.Println("Error on retrieve availabile pkgs: " + err.Error())
					os.Exit(1)
				}

			} else if listMissings {
				if buildOrder {
					list, err = repoList.ListPkgsMissingByDeps(treePath, buildOrderWithResolve)
				} else {
					list, err = repoList.ListPkgsMissing()
				}

				if err != nil {
					fmt.Println("Error on retrieve missings pkgs: " + err.Error())
					os.Exit(1)
				}
			}

			// Filter packages
			if len(filters) > 0 {
				// Create regex
				listRegex := []*regexp.Regexp{}

				for _, f := range filters {
					r := regexp.MustCompile(f)
					if r != nil {
						listRegex = append(listRegex, r)
					} else {
						fmt.Println("WARNING: Regex " + f + " not compiled.")
					}
				}

				if len(listRegex) > 0 {
					filterList := []*anise_pkg.DefaultPackage{}
					for _, p := range list {
						toSkip := true
						for _, r := range listRegex {
							if r.MatchString(p.GetPackageName()) {
								toSkip = false
								break
							}
						}
						if toSkip {
							continue
						}
						filterList = append(filterList, p)
					}

					list = filterList
				}
			}

			if limit > 0 {
				newList := []*anise_pkg.DefaultPackage{}
				for _, p := range list {
					newList = append(newList, p)
					limit -= 1
					if limit <= 0 {
						break
					}
				}

				list = newList
			}

			if jsonOutput {

				listSanitized := []*anise_spectooling.DefaultPackageSanitized{}
				for _, p := range list {
					listSanitized = append(listSanitized, anise_spectooling.NewDefaultPackageSanitized(p))
				}
				// Convert object in sanitized object
				data, _ := json.Marshal(listSanitized)
				fmt.Println(string(data))
			} else {
				orderString := []string{}
				for _, p := range list {
					orderString = append(orderString, p.HumanReadableString())
				}

				if !buildOrder {
					sort.Strings(orderString)
				}

				for _, p := range orderString {
					fmt.Println(p)
				}
			}

		},
	}

	var flags = cmd.Flags()
	flags.StringP("backend", "b", "local", "Select backend repository: local|mottainai|minio.")
	flags.StringP("path", "p", "", "Path of the repository artefacts.")
	flags.String("mottainai-profile", "", "Set mottainai profile to use.")
	flags.String("mottainai-master", "", "Set mottainai Server to use.")
	flags.String("mottainai-apikey", "", "Set mottainai API Key to use.")
	flags.String("mottainai-namespace", "", "Set mottainai namespace to use.")
	flags.String("minio-bucket", "",
		"Set minio bucket to use or set env MINIO_BUCKET.")
	flags.String("minio-endpoint", "",
		"Set minio endpoint to use or set env MINIO_URL.")
	flags.String("minio-keyid", "",
		"Set minio Access Key to use or set env MINIO_ID.")
	flags.String("minio-secret", "",
		"Set minio Access Key to use or set env MINIO_SECRET.")
	flags.String("minio-region", "", "Optinally define the minio region.")
	flags.Bool("availables", false, "Show list of available packages.")
	flags.Bool("missings", false, "Show list of missing packages.")
	flags.Bool("build-ordered", false,
		"Show list of missing packages with a build order. To use with --missings.")
	flags.Bool("build-ordered-with-resolve", false,
		"Use stage4 tree resolving. Slow. To use with --build-ordered.")
	flags.Bool("json", false, "Show packages in JSON format.")
	flags.Int32P("limit", "l", 0, "Limit number of packages returned. 0 means no limit.")
	flags.StringArrayP("filter", "f", []string{},
		"Define one or more regex filter to match packages.")

	return cmd
}
