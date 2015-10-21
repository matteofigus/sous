package nodejs

import (
	"strings"

	"github.com/opentable/sous/config"
	"github.com/opentable/sous/tools/cli"
	"github.com/opentable/sous/tools/docker"
	"github.com/opentable/sous/tools/version"
)

type NodePackage struct {
	Name    string
	Version string
	Engines NodePackageEngines
	Scripts NodePackageScripts
}
type NodePackageEngines struct {
	Node, NPM string
}
type NodePackageScripts struct {
	Start, Test, InstallProduction string
}

func keys(m map[string]string) []string {
	ks := make([]string, len(m))
	i := 0
	for k := range m {
		ks[i] = k
		i++
	}
	return ks
}

var _availableNodeVersions version.VL

func AvailableNodeVersions() version.VL {
	c := config.Load()
	if _availableNodeVersions == nil {
		_availableNodeVersions = version.VersionList(
			keys(c.Packs.NodeJS.NodeVersionsToDockerBaseImages)...)
	}

	return _availableNodeVersions
}

func bestSupportedNodeVersion(np *NodePackage) string {
	var nodeVersion *version.V
	nodeVersion = version.Range(np.Engines.Node).BestMatchFrom(AvailableNodeVersions())
	if nodeVersion == nil {
		cli.Fatalf("unable to satisfy NodeJS version '%s' (from package.json); available versions are: %s",
			np.Engines.Node, strings.Join(AvailableNodeVersions().Strings(), ", "))
	}
	return nodeVersion.String()
}

func dockerFrom(np *NodePackage, nodeVersion string) string {
	c := config.Load()
	return c.Packs.NodeJS.NodeVersionsToDockerBaseImages[nodeVersion]
}

var _config *config.Config

func Config() *config.Config {
	if _config == nil {
		_config = config.Load()
	}
	return _config
}

var npmVersions = version.VersionList("3.3.4", "2.4.15")
var defaultNPMVersion = version.Version("2.4.15")

func npmRegistry() string {
	return Config().Packs.NodeJS.NPMMirrorURL
}

var wd = "/srv/app/"

func baseDockerfile(np *NodePackage) *docker.Dockerfile {
	var Config = config.Load()
	nodeVersion := bestSupportedNodeVersion(np)
	from := dockerFrom(np, nodeVersion)
	npmVer := defaultNPMVersion
	if np.Engines.NPM != "" {
		npmVer = version.Range(np.Engines.NPM).BestMatchFrom(npmVersions)
		if npmVer == nil {
			cli.Logf("NPM version %s not supported, try using a range instead.", np.Engines.NPM)
			cli.Fatalf("Available NPM version ranges are: '^3' and '^2'")
		}
	}
	df := &docker.Dockerfile{
		From:        from,
		Add:         []docker.Add{docker.Add{Files: []string{"."}, Dest: wd}},
		Workdir:     wd,
		LabelPrefix: Config.DockerLabelPrefix,
	}
	npmMajorVer := npmVer.String()[0:1]
	df.AddRun("npm install -g npm@%s", npmMajorVer)
	df.AddLabel("stack.name", "NodeJS")
	df.AddLabel("stack.id", "nodejs")
	df.AddLabel("stack.nodejs.version", nodeVersion)
	return df
}
