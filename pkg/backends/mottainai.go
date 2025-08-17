/*
Copyright © 2020-2025 Macaroni OS Linux
See AUTHORS and LICENSE for the license details and contributors.
*/
package backends

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/macaroni-os/anise-repo-devkit/pkg/specs"

	common "github.com/MottainaiCI/mottainai-server/mottainai-cli/common"
	client "github.com/MottainaiCI/mottainai-server/pkg/client"
	setting "github.com/MottainaiCI/mottainai-server/pkg/settings"
	utils "github.com/MottainaiCI/mottainai-server/pkg/utils"
	schema "github.com/MottainaiCI/mottainai-server/routes/schema"
	v1 "github.com/MottainaiCI/mottainai-server/routes/schema/v1"
	. "github.com/geaaru/luet/pkg/logger"
	artifact "github.com/geaaru/luet/pkg/v2/compiler/types/artifact"
)

type BackendMottainai struct {
	Specs        *specs.AniseRDConfig
	ArtefactPath string

	Config          *setting.Config
	MottainaiClient client.HttpClient
	Namespace       string
}

func setupMottainaiCliConfig(opts map[string]string) (*setting.Config, error) {
	var err error

	config := setting.NewConfig(nil)
	config.Viper.SetEnvPrefix(common.MCLI_ENV_PREFIX)
	config.Viper.BindEnv("config")
	config.Viper.SetDefault("master", "http://localhost:8080")
	config.Viper.SetDefault("profile", "")
	config.Viper.SetDefault("config", "")
	config.Viper.SetDefault("etcd-config", false)

	config.Viper.AutomaticEnv()

	// Set config file name (without extension)
	config.Viper.SetConfigName(common.MCLI_CONFIG_NAME)

	// Set Config paths list
	config.Viper.AddConfigPath(common.MCLI_LOCAL_PATH)
	config.Viper.AddConfigPath(
		fmt.Sprintf("%s/%s", common.GetHomeDir(), common.MCLI_HOME_PATH))

	// Create EnvKey Replacer for handle complex structure
	replacer := strings.NewReplacer(".", "__")
	config.Viper.SetEnvKeyReplacer(replacer)
	config.Viper.SetTypeByDefaultValue(true)

	err = config.Unmarshal()
	if err != nil {
		return nil, err
	}

	configured := false
	if pName, ok := opts["mottainai-profile"]; ok {

		var conf common.ProfileConf
		var profile *common.Profile

		if err = config.Viper.Unmarshal(&conf); err != nil {
			return nil, err
		}

		profile, err = conf.GetProfile(pName)

		if profile != nil {
			config.Viper.Set("master", profile.GetMaster())
			if profile.GetApiKey() != "" {
				config.Viper.Set("apikey", profile.GetApiKey())
			}
		} else {
			return nil,
				errors.New(
					fmt.Sprintf(
						"No profile with name %s. I use default value.\n", pName),
				)
		}

		configured = true

	}

	if !configured {
		if master, ok := opts["mottainai-master"]; ok {
			config.Viper.Set("master", master)
		}

		if apikey, ok := opts["mottainai-apikey"]; ok {
			config.Viper.Set("apikey", apikey)
		}
	}

	return config, nil
}

func NewBackendMottainai(specs *specs.AniseRDConfig, path string, opts map[string]string) (*BackendMottainai, error) {
	if path != "" {
		_, err := os.Stat(path)
		if err != nil {
			return nil, errors.New(
				fmt.Sprintf(
					"Error on retrieve stat of the path %s: %s",
					path, err.Error(),
				))
		}

		if os.IsNotExist(err) {
			return nil, errors.New("The path doesn't exist!")
		}
	}

	if _, ok := opts["mottainai-namespace"]; !ok {
		return nil, errors.New("Mottainai namespace is mandatory")
	}

	config, err := setupMottainaiCliConfig(opts)
	if err != nil {
		return nil, err
	}

	ans := &BackendMottainai{
		Specs:        specs,
		ArtefactPath: path,
		Config:       config,
		MottainaiClient: client.NewTokenClient(
			config.Viper.GetString("master"),
			config.Viper.GetString("apikey"),
			config,
		),
		Namespace: opts["mottainai-namespace"],
	}

	return ans, nil
}

func (b *BackendMottainai) GetFilesList() ([]string, error) {
	var tlist []string
	ans := []string{}

	req := &schema.Request{
		Route:  v1.Schema.GetNamespaceRoute("show_artefacts"),
		Target: &tlist,
		Options: map[string]interface{}{
			":name": b.Namespace,
		},
	}
	err := b.MottainaiClient.Handle(req)
	if err != nil {

		if req.Response != nil {
			Error("HTTP CODE: ", req.Response.StatusCode)
			Error(string(req.ResponseRaw))
			return ans, err
		}
		return ans, err
	}

	// Drop initial slash
	for _, f := range tlist {
		ans = append(ans, f[1:])
	}

	return ans, nil
}

func (b *BackendMottainai) GetMetadata(file string) (*artifact.PackageArtifact, error) {
	var outBuffer bytes.Buffer

	url := b.MottainaiClient.GetBaseURL() +
		path.Join("/namespace/", b.Namespace, utils.PathEscape(file))

	_, err := b.MottainaiClient.DownloadResource(url, &outBuffer,
		b.Config.GetAgent().DownloadRateLimit)
	if err != nil {
		return nil, err
	}

	fileContent := outBuffer.String()

	return artifact.NewPackageArtifactFromYaml([]byte(fileContent))
}

func (b *BackendMottainai) CleanFile(file string) error {
	_, err := b.MottainaiClient.NamespaceRemovePath(b.Namespace,
		"/"+file,
	)
	return err
}
